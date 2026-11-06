// pp:client-call
package awsx

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"aws-billing-pp-cli/internal/cliutil"
)

// metricUnblended is the cost metric the CLI reports throughout. UnblendedCost
// is the on-demand-equivalent figure that matches what most people see on the
// bill; it avoids the amortization surprises of blended/amortized metrics.
const metricUnblended = "UnblendedCost"

// ceCall paces a Cost Explorer request through the adaptive limiter and maps a
// throttle response to a typed cliutil.RateLimitError so callers never read a
// throttle as "no data".
func (c *Client) ceCall(ctx context.Context, label string, fn func() error) error {
	if err := c.ceLimiter.Wait(ctx); err != nil {
		return err
	}
	err := fn()
	if err != nil {
		if IsThrottle(err) {
			c.ceLimiter.OnRateLimit()
			return &cliutil.RateLimitError{URL: "cost-explorer:" + label, Body: err.Error()}
		}
		return err
	}
	c.ceLimiter.OnSuccess()
	return nil
}

// CostBucket is one time period of a Cost Explorer answer (ResultsByTime
// TimePeriod: Start inclusive, End exclusive, YYYY-MM-DD). Estimated is
// ResultsByTime Estimated: the period's amounts are not final yet (the
// current month), so a stored copy goes stale.
type CostBucket struct {
	Start     string
	End       string
	Estimated bool
}

// CostAnswer is one complete GetCostAndUsage answer: every group of every
// page, and every time period AWS answered for, a period with no groups (no
// spend) included (aws-billing handfix cost-cache-answers-only-its-request:
// the local cache stores and reuses a period only as AWS answered it).
type CostAnswer struct {
	Lines   []CostLine
	Buckets []CostBucket
}

// GetCostAndUsageGrouped returns monthly cost lines grouped by the given
// dimensions (1 or 2). groupKeys are CE dimension keys such as
// "LINKED_ACCOUNT", "SERVICE", "USAGE_TYPE", "REGION". start/end are
// YYYY-MM-DD; end is exclusive. The returned lines carry the dimension values
// in order: groupKeys[0] -> first field, groupKeys[1] -> second.
func (c *Client) GetCostAndUsageGrouped(ctx context.Context, start, end, granularity string, groupKeys []string) ([]CostLine, error) {
	ans, err := c.GetCostAndUsageAnswer(ctx, start, end, granularity, groupKeys)
	return ans.Lines, err
}

// GetCostAndUsageAnswer reads every NextPageToken page of one
// GetCostAndUsage request. A group repeated for one period across pages is
// summed into one line, so the answer has one line per period and group key.
func (c *Client) GetCostAndUsageAnswer(ctx context.Context, start, end, granularity string, groupKeys []string) (CostAnswer, error) {
	groups := make([]cetypes.GroupDefinition, 0, len(groupKeys))
	for _, k := range groupKeys {
		groups = append(groups, cetypes.GroupDefinition{
			Type: cetypes.GroupDefinitionTypeDimension,
			Key:  aws.String(k),
		})
	}
	gran := cetypes.GranularityMonthly
	if granularity == "DAILY" {
		gran = cetypes.GranularityDaily
	}

	var ans CostAnswer
	bucketAt := map[[2]string]int{}
	lineAt := map[string]int{}
	var nextToken *string
	for {
		in := &costexplorer.GetCostAndUsageInput{
			TimePeriod:  &cetypes.DateInterval{Start: aws.String(start), End: aws.String(end)},
			Granularity: gran,
			Metrics:     []string{metricUnblended},
			GroupBy:     groups,
		}
		if nextToken != nil {
			in.NextPageToken = nextToken
		}
		var out *costexplorer.GetCostAndUsageOutput
		if err := c.ceCall(ctx, "GetCostAndUsage", func() error {
			var e error
			out, e = c.CE.GetCostAndUsage(ctx, in)
			return e
		}); err != nil {
			return CostAnswer{}, err
		}
		for _, rbt := range out.ResultsByTime {
			var ps, pe string
			if rbt.TimePeriod != nil {
				ps, pe = aws.ToString(rbt.TimePeriod.Start), aws.ToString(rbt.TimePeriod.End)
			}
			if ps != "" && pe != "" {
				if i, seen := bucketAt[[2]string{ps, pe}]; seen {
					ans.Buckets[i].Estimated = ans.Buckets[i].Estimated || rbt.Estimated // any page saying estimated wins
				} else {
					bucketAt[[2]string{ps, pe}] = len(ans.Buckets)
					ans.Buckets = append(ans.Buckets, CostBucket{Start: ps, End: pe, Estimated: rbt.Estimated})
				}
			}
			for _, g := range rbt.Groups {
				amount := 0.0
				unit := "USD"
				if mv, ok := g.Metrics[metricUnblended]; ok {
					amount = parseAmount(mv.Amount)
					if mv.Unit != nil {
						unit = aws.ToString(mv.Unit)
					}
				}
				line := CostLine{
					PeriodStart: ps,
					PeriodEnd:   pe,
					Granularity: string(gran),
					AmountUSD:   amount,
					Unit:        unit,
				}
				assignGroupKeys(&line, groupKeys, g.Keys)
				key := ps + "\x00" + pe + "\x00" + strings.Join(g.Keys, "\x00")
				if i, dup := lineAt[key]; dup {
					ans.Lines[i].AmountUSD += amount
					continue
				}
				lineAt[key] = len(ans.Lines)
				ans.Lines = append(ans.Lines, line)
			}
		}
		if out.NextPageToken == nil || aws.ToString(out.NextPageToken) == "" {
			break
		}
		nextToken = out.NextPageToken
	}
	return ans, nil
}

// assignGroupKeys maps CE group dimension values onto the CostLine fields.
func assignGroupKeys(line *CostLine, groupKeys, values []string) {
	for i, k := range groupKeys {
		if i >= len(values) {
			break
		}
		v := values[i]
		switch k {
		case "LINKED_ACCOUNT":
			line.AccountID = v
		case "SERVICE":
			line.Service = v
		case "USAGE_TYPE":
			line.UsageType = v
		case "REGION":
			line.Region = v
		}
	}
}

// GetCostForecast returns the forecasted spend for [start,end) (YYYY-MM-DD).
// Forecasts require at least some historical usage; AWS returns an error when
// there isn't enough history, which the caller surfaces as a soft note.
func (c *Client) GetCostForecast(ctx context.Context, start, end, granularity string) (Forecast, error) {
	gran := cetypes.GranularityMonthly
	if granularity == "DAILY" {
		gran = cetypes.GranularityDaily
	}
	var out *costexplorer.GetCostForecastOutput
	err := c.ceCall(ctx, "GetCostForecast", func() error {
		var e error
		out, e = c.CE.GetCostForecast(ctx, &costexplorer.GetCostForecastInput{
			TimePeriod:              &cetypes.DateInterval{Start: aws.String(start), End: aws.String(end)},
			Granularity:             gran,
			Metric:                  cetypes.MetricUnblendedCost,
			PredictionIntervalLevel: aws.Int32(80),
		})
		return e
	})
	if err != nil {
		return Forecast{}, err
	}
	f := Forecast{PeriodStart: start, PeriodEnd: end}
	if out.Total != nil {
		f.MeanUSD = parseAmount(out.Total.Amount)
	}
	if len(out.ForecastResultsByTime) > 0 {
		first := out.ForecastResultsByTime[0]
		f.LowerUSD = parseAmount(first.PredictionIntervalLowerBound)
		f.UpperUSD = parseAmount(first.PredictionIntervalUpperBound)
	}
	return f, nil
}

// GetDimensionValues lists the distinct values for a Cost Explorer dimension
// over [start,end). dimension is e.g. "SERVICE", "LINKED_ACCOUNT", "REGION",
// "USAGE_TYPE".
func (c *Client) GetDimensionValues(ctx context.Context, start, end, dimension string) ([]DimensionValue, error) {
	// aws-billing handfix (ce-dimension-values-paged): GetDimensionValues
	// answers in pages (NextPageToken); every page is read.
	var vals []DimensionValue
	var next *string
	for {
		var out *costexplorer.GetDimensionValuesOutput
		err := c.ceCall(ctx, "GetDimensionValues", func() error {
			var e error
			out, e = c.CE.GetDimensionValues(ctx, &costexplorer.GetDimensionValuesInput{
				TimePeriod:    &cetypes.DateInterval{Start: aws.String(start), End: aws.String(end)},
				Dimension:     cetypes.Dimension(dimension),
				NextPageToken: next,
			})
			return e
		})
		if err != nil {
			return nil, err
		}
		for _, dv := range out.DimensionValues {
			vals = append(vals, DimensionValue{
				Value:     aws.ToString(dv.Value),
				Dimension: dimension,
			})
		}
		if aws.ToString(out.NextPageToken) == "" {
			return vals, nil
		}
		next = out.NextPageToken
	}
}
