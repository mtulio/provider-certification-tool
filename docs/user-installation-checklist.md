# OPCT - Cluster Installation Checklist

> WIP - this document is working in progress

<!--
Do not change the following markdown commented lines.
__version__: 0.1.0-preview
-->

> Checklist that should be provided on the first submission, when opening the support case.

This checklist should be provided every new support case, or when any items have been changed (Example, Compute flavor).

if you have any questions you can:

- Review the [Installation Review Guide](./user-installation-review.md)
- Review the [OpenShift Infrastructure Provider Guide](https://docs.providers.openshift.org/)
- Review the [OpenShift Documentation Page related the version your are certifying](https://docs.openshift.com/container-platform)
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

### Load Balancers

- [ ] I have reviewed all the Health Check requirements
- [ ] The Health Checks for KAS listeners are HTTP or HTTPS
- [ ] I have reviewed the Hairpin connection problem, and the Load Balancer used to kubernetes-api is not impacted by it

- Load Balancer flavor/type used by kubernetes API:
- Load Balancer flavor/type used by Default Ingress:

### Component specific Review

#### etcd

- [ ] I have checked the etcd logs while running the certification tool

> TODO: provide an example of how to check it. (link to the "Installation Review" document)

- [ ] I ran the etcd performance tool to measure the performance of the disk used by the mount point used by etcd(`/var/lib/etcd`) and it reported below 20 ms (milliseconds).

> TODO: link to the "Installation Review" document (and to KCS)

#### image-registry

- Persistent storage used on the internal image registry: 

- [ ] I am able to push the image to the registry
- [ ] I am able to pull images from the registry
- [ ] I am able to create resources (deployment) with custom images

> TODO: link to the "Installation Review" document
