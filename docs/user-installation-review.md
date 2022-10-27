# OpenShift Provider Certification Tool - Installation Review

> WIP - this document is working in progress

> TODO: steps describing in detail the topics to review before submitting the results.

## Compute

> TODO: any compute specific?

- Minimal required for Compute nodes: [User Documentation -> Pre-requisites](./user.md#prerequisites)

## Load Balancers

- Private Load Balancers

The basic OpenShift Installations with support of external Load Balancers are used to deploy 3 Load Balancers: public and private for control plane services (Kubernetes API and Machine Config Server), and one public for the ingress.

The address for the private Load Balancer must point to the DNS `api-int.<cluster>.<domain>`, which will be accessed for internal services.

Reference: [User-provisioned DNS requirements](https://docs.openshift.com/container-platform/4.11/installing/installing_platform_agnostic/installing-platform-agnostic.html#installation-dns-user-infra_installing-platform-agnostic)

### Health Checks

The kube-apiserver has a graceful termination engine that requires the Load Balancer health check probe to HTTP path.

| Service | Protocol | Port | Path | Threshold | Interval | Timeout |
| -- | -- | -- | -- | -- | -- | -- |
| Kubernetes API Server | HTTPS* | 6443 | /readyz | 2  | 10 | 10 |
| Machine Config Server | HTTPS* | 22623 | /healthz | 2  | 10 | 10 |
| Ingress | TCP | 80 | - | 2  | 10 | 10 |
| Ingress | TCP | 443 | - | 2  | 10 | 10 |

> Note/Question: Not sure if we need to keep the HTTP (non-SSL on the doc). In the past, I talked with KAS team and he had plans to remove that option, but due to the limitation of a few cloud providers it will not. Some providers that still use this: [Alibaba](https://github.com/openshift/installer/blob/master/data/data/alibabacloud/cluster/vpc/slb.tf#L31), [GCP Public](https://github.com/openshift/installer/blob/master/data/data/gcp/cluster/network/lb-public.tf#L20-L21)
*It's required to health check support HTTP protocol. If the Load Balancer used does not support SSL, alternatively and not preferably you can use HTTP - but never TCP:

| Service | Protocol | Port | Path | Threshold | Interval | Timeout |
| -- | -- | -- | -- | -- | -- | -- |
| Kubernetes API Server | HTTP* | 6080 | /readyz | 2  | 10 | 10 |
| Machine Config Server | HTTP* | 22624 | /healthz | 2  | 10 | 10 |


- Flavor/Size

> TODO: Need to check if we have any information in our documentation. I am considering the current deployments of Alibaba and NLB (auto-scaling)

The Load Balancer used by the Kubernetes API must support the throughput higher than 100Mbp/s

Reference:

* [AWS](https://github.com/openshift/installer/blob/master/data/data/aws/cluster/vpc/master-elb.tf#L3): NLB (Network Load Balancer)
* [Alibaba](https://github.com/openshift/installer/blob/master/data/data/alibabacloud/cluster/vpc/slb.tf#L49): `slb.s2.small`
* [Azure]()https://github.com/openshift/installer/blob/master/data/data/azure/vnet/internal-lb.tf#L7: Standard


### The Load Balancer and the Hairpin traffic

If the Load Balancer does not support hairpin traffic, when a backend is load-balanced to itself and the traffic is dropped, you need to provide a solution.

On the integrated clouds that do not support Hairpin traffic, the OpenShift provides one static pod to redirect traffic destined to the lb VIP back to the node on the kube-apiserver.

Reference:

- [Static pods to redirect hairpin traffic for Azure](https://github.com/openshift/machine-config-operator/blob/master/templates/master/00-master/azure/files/opt-libexec-openshift-azure-routes-sh.yaml)
- [Static pods to redirect hairpin traffic for AlibabaCloud](https://github.com/openshift/machine-config-operator/tree/master/templates/master/00-master/alibabacloud)


## Components

### etcd

Review etcd's disk speed requirements:

- https://etcd.io/docs/v3.5/op-guide/hardware/
- https://docs.openshift.com/container-platform/4.11/scalability_and_performance/planning-your-environment-according-to-object-maximums.html
- https://www.ibm.com/cloud/blog/using-fio-to-tell-whether-your-storage-is-fast-enough-for-etcd
- Backend Performance Requirements for OpenShift etcd: https://access.redhat.com/solutions/4770281

#### Run OpenShift etcd-fio tests

The [KCS "How to Use 'fio' to Check Etcd Disk Performance in OCP"](https://access.redhat.com/solutions/4885641) is a guide to check if the disk used by etcd has the expected performance on OpenShift.

<!-- #### Run dense FIO tests

> Note: Keep this section commented as we don't have a strong need to implement or share this broadly.

This section documents how to run dense disk tests using `fio`.

> References:
- https://fio.readthedocs.io/en/latest/fio_doc.html
- https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/benchmark_procedures.html
- https://cloud.google.com/compute/docs/disks/benchmarking-pd-performance
-->

#### Review etcd logs: etcd slow requests

This section provides a guide to check the etcd slow requests from the logs on the etcd pods to understand how the etcd is performing while running the e2e tests.

The steps below use a utility `insights-ocp-etcd-logs` to parse the logs, aggregate the requests into buckets of 100ms from 200ms to 1s and report it on the stdout.

This is the utility to help you to troubleshoot the slow requests into your cluster, and help to take some decisions like changing the flavor of the block device used by the control plane, increasing IOPS, changing the flavor of the instances, etc.

There's no magic or desired number, but for reference, based on the observations from integrated platforms, is to have no more than 30-40% of requests above 500ms while running the certification tests.

> TODO: provide guidance on how to get the errors from the etcd pods, and parse it into buckets of latency to understand the performance of the etcd while running the certification environment.

- Export the location you must-gather has been extracted:

```bash
export MUST_GATHER_PATH=${PWD}/must-gather.local.2905984348081335046
```

- Extract the utility from the tools repository:

```bash
TOOLS_IMAGE=$(skopeo inspect docker://quay.io/ocp-cert/tools:latest | jq .Digest)
oc image extract ${TOOLS_IMAGE} --file="/usr/bin/insights-ocp-etcd-logs"
chmod u+x insights-ocp-etcd-logs
```

- Generate the overall report

> Note: This report can be useless depending on the history of logs. We recommend looking the next report which aggregate by the hour, so you can check the time frame the certification has been executed

> TODO: the tool `insights-ocp-etcd-logs` could read all the logs from stdin, filter by str, then report the buckets - avoiding extracting complex greps

```bash
grep -rni "apply request took too long" ${MUST_GATHER_PATH} \
    | grep -Po 'took":"([a-z0-9\.]+)"' \
    | awk -F'took":' '{print$2}' \
    | tr -d '"' \
    | ./insights-ocp-etcd-logs
```

- Generate the overall report aggregated by the hour:

> TODO: also improve the tool `insights-ocp-etcd-logs` to report by hour - maybe using flags

```bash
FILTER_MSG="apply request took too long"
for TS in $( grep -rni "${FILTER_MSG}" ${MUST_GATHER_PATH} \
    | awk '{print$1}' \
    | awk -F'.log:' '{print$2}' \
    | awk -F':' '{print$2}' \
    | sort | uniq); do
    echo "-> ${TS}"
    grep -rni "${FILTER_MSG}" ${MUST_GATHER_PATH} \
        | grep $TS \
        | grep -Po 'took":"([a-z0-9\.]+)"' \
        | awk -F'took":' '{print$2}' \
        | tr -d '"' \
        | ./insights-ocp-etcd-logs
done
```

#### Mount /var/lib/etcd in sepparate disk

One way to improve the performance on etcd is to use a dedicated block device.

You can mount `/var/lib/etcd` by following the documentation:

- [OpenShift Docs: Disk partitioning](https://docs.openshift.com/container-platform/4.11/installing/installing_bare_metal/installing-bare-metal.html#installation-user-infra-machines-advanced_disk_installing-bare-metal)
- [KCS: Mounting separate disk for OpenShift 4 etcd](https://access.redhat.com/solutions/5840061)

### Image Registry

- persistent storage defined for image-registry

- make sure you can write on the image-registry

> TODO: describe steps to write to the registry

- make sure you can use custom images from the image-registry

> TODO: describe steps to create a deployment using a custom image


## <Open>

> Question: Anything else related to provider review findings that must be checked before submitting the results?
