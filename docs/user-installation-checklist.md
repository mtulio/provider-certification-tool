# Cluster Installation Check List

> WIP: this document is working in progresss

> Check list that should be provided on the first submission, when opening the support case.

## Review Check list

### Compute

- [ ] The Control Plane nodes meet the minimum requirements
- [ ] The Compute nodes meet the minimum requirements

- Control Plane pool Flavor:
- Compute pool Flavor:
- Link with the details of the flavors:

### Load Balancers

- [ ] I have reviewed all the Health Check requirements
- [ ] The Health Check for KAS are HTTP or HTTPS
- [ ] I have reviewed the Hairpin connection problem, and the Load Balancer used to kubernetes-api is not impacted by it

- Load Balancer flavor/type use by kubernetes API:
- Load Balancer flavor/type use by Default Ingress:

### Component specific Review

#### etcd

- [ ] I checked the etcd logs while running the certification tool

> TODO provide an example how to check it

#### image-registry

- Persistent storage used on the internal image registry: 

- [ ] I am able to push image to the registry
- [ ] I am able to pull image from the registry
- [ ] I am able to create resources with custom images
> TODO provide an example how to do it
