# OPCT - Cluster Installation Checklist

> WIP - this document is working in progress

<!--
Do not change the following markdown commented lines.
__version__: 0.1.0-preview
-->

This checklist should be provided for every new support case, or when any items have been changed (for Example, Compute flavor). This will help the Red Hat engineers while working in your partner support case during the certification process.

If you have any questions you can:

- Review the [Installation Review Guide](./user-installation-review.md)
- Review the [OpenShift Infrastructure Provider Guide](https://docs.providers.openshift.org/)
- Review the [OpenShift Documentation Page related to the version your are certifying](https://docs.openshift.com/container-platform)
- Ask your Red Hat Partner

## Provider Information

- Provider Name:                # Example: MyCloud
- OpenShift Version:            # Example: 4.11.4
- Certification Type:           # Options: (Standard | Upgrade)
- Environment Setup Topology:   # Options: (Standard | Dedicated)

## Review Checklist

### Compute

- [ ] The Control Plane nodes meet the minimum requirements
- [ ] The Compute nodes meet the minimum requirements

- Control Plane pool flavor:
- Compute pool flavor:
- Public documentation with the details of the flavor offering:

*See more at [User Installation Review > Compute](./user-installation-review.md#compute)*

### Load Balancers

- [ ] I have reviewed all the Health Check requirements
- [ ] The Health Checks for KAS listeners are HTTP or HTTPS
- [ ] The DNS `api-int.<cluster>.<domain>` is properly configured with **private** Load Balancer address
- [ ] I have reviewed the Hairpin connection problem, and the Load Balancer used to kubernetes-api is not impacted by it

- Load Balancer flavor/type used by kubernetes API:
- Load Balancer flavor/type used by Default Ingress:


*See more at [User Installation Review > Load Balancers](./user-installation-review.md#load-balancers)*

### Component-specific Review

#### etcd

- [ ] I have checked the etcd logs while running the certification tool

> TODO: provide an example of how to check it. (link to the "Installation Review" document)

- [ ] I ran the etcd performance tool to measure the performance of the disk used by the mount point used by etcd(`/var/lib/etcd`) and it reported below than 10 ms (milliseconds).

*See more at [User Installation Review > Components > etcd](./user-installation-review.md#components-etcd)*

#### image-registry

- Persistent storage used on the internal image registry: 

- [ ] I can push the image to the registry
- [ ] I can pull images from the registry
- [ ] I can create resources (deployment) with custom images

*See more at [User Installation Review > Components > image-registry](./user-installation-review.md#components-imageregistry)*
