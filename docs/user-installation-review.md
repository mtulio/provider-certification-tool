# OpenShift Provider Certification Tool - Installation Review

> WIP - this document is working in progress

> TODO: steps describing in detail the topics to review before submitting the results.

## Compute

- minimal required:

> Link to the Official Documentation


## Load Balancers

- make sure the Health checks are properly set HTTP or HTTPS
- check the flavor/size
- check the internal facing option
- check the hairpin connection problem

## Components

### etcd

The etcd is very disk sensitive (...)

#### Review etcd logs: etcd slow requests

> TODO: provide guidance on how to get the errors from the etcd pods, and parse it into buckets of latency to understand the performance of the etcd while running the certification environment.

#### Run etcd-fio tests

> TODO: Link to KCS and provide the example and expected results

#### Run FIO tests

> Note: sometimes the single test

> TODO: Provide the example and expected results

#### Alternative to mount /var/lib/etcd in sepparate disk

> TODO: Need to check if it's supported in UPI deployments

> NOTE: it has proven that will provide better performance to etcd when providing a dedicated disk, and not sharing IO with OS and many other components on the OS-disk

> TODO: link with internal and public studies

### Image Registry

- persistent storage defined for image-registry

- make sure you can write on the image-registry

> TODO: describe steps to write to the registry

- make sure you can use custom images from the image-registry

> TODO: describe steps to create a deployment using a custom image
