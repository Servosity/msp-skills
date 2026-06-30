// pp:client-call
package awsx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// idleCPUThreshold is the average CPU% below which a running instance is
// flagged as idle.
const idleCPUThreshold = 5.0

// InventoryOptions bounds the work an inventory scan does. SkipCPU avoids the
// per-instance CloudWatch calls (used under dogfood to stay within the 30s cap).
type InventoryOptions struct {
	SkipCPU bool
	CPUDays int // trailing window for idle detection; default 14
}

// CollectInventory walks EC2 instances, EBS volumes, snapshots, Elastic IPs,
// and S3 buckets in the current account/region, flagging waste candidates with
// an estimated monthly dollar figure. Read-only.
func (c *Client) CollectInventory(ctx context.Context, opts InventoryOptions) ([]InventoryResource, error) {
	if opts.CPUDays <= 0 {
		opts.CPUDays = 14
	}
	account, _, _ := c.CallerIdentity(ctx)

	var out []InventoryResource

	volumeIDs := map[string]bool{}

	// EBS volumes
	vols, err := c.collectVolumes(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("describe volumes: %w", err)
	}
	for _, v := range vols {
		if v.ResourceID != "" {
			volumeIDs[v.ResourceID] = true
		}
	}
	out = append(out, vols...)

	// Snapshots (owned by this account) — orphaned if source volume is gone.
	snaps, err := c.collectSnapshots(ctx, account, volumeIDs)
	if err != nil {
		return nil, fmt.Errorf("describe snapshots: %w", err)
	}
	out = append(out, snaps...)

	// Elastic IPs
	eips, err := c.collectAddresses(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("describe addresses: %w", err)
	}
	out = append(out, eips...)

	// EC2 instances (idle detection via CloudWatch unless skipped)
	insts, err := c.collectInstances(ctx, account, opts)
	if err != nil {
		return nil, fmt.Errorf("describe instances: %w", err)
	}
	out = append(out, insts...)

	// S3 buckets (informational inventory; size best-effort)
	buckets, err := c.collectBuckets(ctx, account)
	if err == nil { // S3 list is best-effort; don't fail the whole scan
		out = append(out, buckets...)
	}

	return out, nil
}

func tagsToJSON(tags map[string]string) string {
	if len(tags) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func ec2TagMap(tags []ec2types.Tag) map[string]string {
	m := map[string]string{}
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

func (c *Client) collectVolumes(ctx context.Context, account string) ([]InventoryResource, error) {
	var res []InventoryResource
	p := ec2.NewDescribeVolumesPaginator(c.EC2, &ec2.DescribeVolumesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return res, err
		}
		for _, v := range page.Volumes {
			id := aws.ToString(v.VolumeId)
			volType := string(v.VolumeType)
			size := float64(aws.ToInt32(v.Size))
			cost := size * ebsPerGBMonth(volType)
			tags := ec2TagMap(v.Tags)
			r := InventoryResource{
				ResourceType:   "ebs",
				ResourceID:     id,
				AccountID:      account,
				Region:         c.Region,
				State:          string(v.State),
				MonthlyCostUSD: round2(cost),
				Attrs:          tagsToJSON(mergeAttrs(tags, map[string]string{"volume_type": volType, "size_gb": fmt.Sprintf("%.0f", size)})),
			}
			switch v.State {
			case ec2types.VolumeStateAvailable:
				// Unattached volume — full cost is waste.
				r.MonthlyWasteUSD = round2(cost)
				r.WasteReason = "unattached EBS volume (not connected to any instance)"
			default:
				if strings.EqualFold(volType, "gp2") {
					// gp2->gp3 modernization candidate: waste is the delta.
					delta := size * (ebsGP2PerGBMonth - ebsGP3PerGBMonth)
					r.MonthlyWasteUSD = round2(delta)
					r.WasteReason = "gp2 volume — convert to gp3 to save"
				}
			}
			res = append(res, r)
		}
	}
	return res, nil
}

func (c *Client) collectSnapshots(ctx context.Context, account string, liveVolumes map[string]bool) ([]InventoryResource, error) {
	var res []InventoryResource
	p := ec2.NewDescribeSnapshotsPaginator(c.EC2, &ec2.DescribeSnapshotsInput{
		OwnerIds: []string{"self"},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return res, err
		}
		for _, s := range page.Snapshots {
			id := aws.ToString(s.SnapshotId)
			size := float64(aws.ToInt32(s.VolumeSize))
			cost := size * snapshotPerGBMonth
			srcVol := aws.ToString(s.VolumeId)
			tags := ec2TagMap(s.Tags)
			started := aws.ToTime(s.StartTime)
			ageDays := int(time.Since(started).Hours() / 24)
			r := InventoryResource{
				ResourceType:   "snapshot",
				ResourceID:     id,
				AccountID:      account,
				Region:         c.Region,
				State:          string(s.State),
				MonthlyCostUSD: round2(cost),
				Attrs: tagsToJSON(mergeAttrs(tags, map[string]string{
					"source_volume": srcVol, "size_gb": fmt.Sprintf("%.0f", size), "age_days": fmt.Sprintf("%d", ageDays),
				})),
			}
			orphaned := srcVol != "" && srcVol != "vol-ffffffff" && !liveVolumes[srcVol]
			switch {
			case orphaned:
				r.MonthlyWasteUSD = round2(cost)
				r.WasteReason = "orphaned snapshot (source volume no longer exists)"
			case ageDays > 365:
				r.MonthlyWasteUSD = round2(cost)
				r.WasteReason = fmt.Sprintf("snapshot older than 1 year (%d days) — review for deletion", ageDays)
			}
			res = append(res, r)
		}
	}
	return res, nil
}

func (c *Client) collectAddresses(ctx context.Context, account string) ([]InventoryResource, error) {
	out, err := c.EC2.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, err
	}
	var res []InventoryResource
	for _, a := range out.Addresses {
		id := aws.ToString(a.AllocationId)
		if id == "" {
			id = aws.ToString(a.PublicIp)
		}
		associated := aws.ToString(a.AssociationId) != "" || aws.ToString(a.InstanceId) != ""
		r := InventoryResource{
			ResourceType:   "eip",
			ResourceID:     id,
			AccountID:      account,
			Region:         c.Region,
			MonthlyCostUSD: round2(eipPerMonth),
			Attrs:          tagsToJSON(mergeAttrs(ec2TagMap(a.Tags), map[string]string{"public_ip": aws.ToString(a.PublicIp)})),
		}
		if associated {
			r.State = "associated"
		} else {
			r.State = "unassociated"
			r.MonthlyWasteUSD = round2(eipPerMonth)
			r.WasteReason = "unassociated Elastic IP (billed while not attached)"
		}
		res = append(res, r)
	}
	return res, nil
}

func (c *Client) collectInstances(ctx context.Context, account string, opts InventoryOptions) ([]InventoryResource, error) {
	var res []InventoryResource
	p := ec2.NewDescribeInstancesPaginator(c.EC2, &ec2.DescribeInstancesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return res, err
		}
		for _, rsv := range page.Reservations {
			for _, inst := range rsv.Instances {
				id := aws.ToString(inst.InstanceId)
				itype := string(inst.InstanceType)
				state := ""
				if inst.State != nil {
					state = string(inst.State.Name)
				}
				monthly, known := ec2OnDemandMonthly(itype)
				tags := ec2TagMap(inst.Tags)
				r := InventoryResource{
					ResourceType:   "ec2",
					ResourceID:     id,
					AccountID:      account,
					Region:         c.Region,
					State:          state,
					MonthlyCostUSD: round2(monthly),
					Attrs:          tagsToJSON(mergeAttrs(tags, map[string]string{"instance_type": itype, "price_known": fmt.Sprintf("%t", known)})),
				}
				if state == "running" && !opts.SkipCPU {
					avg, ok := c.avgCPU(ctx, id, opts.CPUDays)
					if ok && avg < idleCPUThreshold {
						if known {
							r.MonthlyWasteUSD = round2(monthly)
						}
						r.WasteReason = fmt.Sprintf("idle instance: %.1f%% avg CPU over %dd (stop or rightsize)", avg, opts.CPUDays)
					}
				}
				res = append(res, r)
			}
		}
	}
	return res, nil
}

// avgCPU returns the average CPUUtilization for an instance over the trailing
// window. ok=false when no datapoints (instance too new or metrics off).
func (c *Client) avgCPU(ctx context.Context, instanceID string, days int) (float64, bool) {
	end := time.Now().UTC()
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	out, err := c.CW.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []cwtypes.Dimension{{Name: aws.String("InstanceId"), Value: aws.String(instanceID)}},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
	})
	if err != nil || len(out.Datapoints) == 0 {
		return 0, false
	}
	var sum float64
	for _, dp := range out.Datapoints {
		sum += aws.ToFloat64(dp.Average)
	}
	return sum / float64(len(out.Datapoints)), true
}

func (c *Client) collectBuckets(ctx context.Context, account string) ([]InventoryResource, error) {
	out, err := c.S3.ListBuckets(ctx, nil)
	if err != nil {
		return nil, err
	}
	var res []InventoryResource
	for _, b := range out.Buckets {
		name := aws.ToString(b.Name)
		res = append(res, InventoryResource{
			ResourceType:   "s3",
			ResourceID:     name,
			AccountID:      account,
			Region:         c.Region,
			State:          "active",
			MonthlyCostUSD: 0, // size-based cost requires per-bucket CloudWatch; informational only
			Attrs:          tagsToJSON(map[string]string{"created": aws.ToTime(b.CreationDate).Format("2006-01-02")}),
		})
	}
	return res, nil
}

func mergeAttrs(base map[string]string, extra map[string]string) map[string]string {
	if base == nil {
		base = map[string]string{}
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
