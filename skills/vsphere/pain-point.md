# vsphere skill - the MSP pain it closes

## The pain

Broadcom moved VMware to per-core subscriptions, and many MSPs saw their own and
their clients' bills multiply. ServeTheHome reported cost increases of 10x or more
for VMware cloud service providers
([ServeTheHome, February 2024](https://www.servethehome.com/vmware-vcsp-customers-seeing-10x-or-more-cost-increases-under-broadcom/)), and Broadcom
now counts at least 16 cores per CPU ([Broadcom KB 313548](https://knowledge.broadcom.com/external/article/313548)).
Every client with a vCenter now asks two questions: what does staying cost, and
what does leaving take?

Answering them today means running RVTools on a Windows jump box, exporting a
spreadsheet per vCenter, and spending an afternoon sorting tabs by hand to find
snapshots, mounted ISOs, PVSCSI controllers and missing VMware Tools, then doing
the core math in another sheet.

## What this skill does about it

- `vsphere-cli migration blockers --severity block --agent`: every VM that cannot
  move yet, with a reason code for each blocker.
- `vsphere-cli licence exposure --price-per-core 350 --agent`: licensable cores per
  host, cluster and vCenter under the 16-core minimum, with an estimate in your price.
- `vsphere-cli snapshots audit --older-than 7d --agent`: forgotten snapshots, their
  age, chain depth and delta-disk bytes, before they stall a cutover.
- `vsphere-cli migration capacity --powered-on-only --agent`: the vCPU, RAM and
  storage the Proxmox target needs.
- `vsphere-cli rvtools export --out ./acme-rvtools`: the RVTools CSV layout from
  macOS or Linux, no Windows box needed.

## Status

Beta. Validated offline against the VMware vSphere API surface; the closed-loop
receipt (a named MSP running it live against their own vCenter at a Build Session)
is tracked separately and added here as `video.md` once it exists.
