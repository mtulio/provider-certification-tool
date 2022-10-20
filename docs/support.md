# OpenShift Provider Certification Tool - Support Guide


## Support Case Check List

### Initial

Check-list to require when new support case have beem opened:

- Documentation: Installing Steps containg the flavors/size of the Infrastructure and the steps to install OCP
- Documentation: Diagram of the Architecture including zonal deployment
- Archive with Certification results
- Archie with must-gather

### New Executions

The following assets, certification assets, should be updated when certain conditions happend:

- Certification Results
- Must Gather
- Install Documentation (when any item/flavor/configuration have beem modified)


The following conditions requires new data:

- OpenShift Container Platform has been updated
- Any Infrastructure component (e.g.: server size, disk category, ELB type/size/config) or cluster dependencies (e.g.: external storage backend for image registry) have beem modified


## Review Environment

### Setting up local environment

- Download the OPCT
- Download the omg

### Download dependencies

- Download the Baseline execution for the version used by partner
- Download the suite test list fo the version used by partner

### Download Partner Results

- Download the Provider certification archive
- Download the Must-gather

### Extract Data

Compare the provider results with the baseline;

```bash
./openshift-provider-cert-linux-amd64 process \
    --baseline opct_baseline-ocp_4.11.4-platform_none-hw-date_uuid.tar.gz \
    --base-suite-ocp ./test-list_openshift-tests_openshift-validated.txt \
    --base-suite-k8s ./test-list_openshift-tests_kubernetes-conformance.txt \
    202210132151_sonobuoy_6af99324-2dc6-4de4-938c-200b84111481.tar.gz
```

Compare the results AND extract the files to local directory `./results-provider-processed`

```bash
./openshift-provider-cert-linux-amd64 process \
    --baseline opct_baseline-ocp_4.11.4-platform_none-hw-date_uuid.tar.gz \
    --base-suite-ocp ./test-list_openshift-tests_openshift-validated.txt \
    --base-suite-k8s ./test-list_openshift-tests_kubernetes-conformance.txt \
    --save-to processed \
    202210132151_sonobuoy_6af99324-2dc6-4de4-938c-200b84111481.tar.gz
```

### Understanding the Results (stdout)

- Header

> TODO: the tabulation is not ok when pasting to Markdown

- Processed Summary (with example results)
```bash
(...Header...)

> Processed Summary <

 Total Tests suites:
 - kubernetes/conformance: 353
 - openshift/conformance: 3488

 Total Tests by Certification Layer:
 - openshift-kube-conformance:
   - Status: failed
   - Total: 675
   - Passed: 654
   - Failed: 21
   - Timeout: 0
   - Skipped: 0
   - Failed (without filters) : 21
   - Failed (Filter SuiteOnly): 2
   - Failed (Filter Baseline  : 2
   - Failed (Filter CI Flakes): 0
   - Status After Filters     : pass
 - openshift-conformance-validated:
   - Status: failed
   - Total: 3818
   - Passed: 1708
   - Failed: 61
   - Timeout: 0
   - Skipped: 2049
   - Failed (without filters) : 61
   - Failed (Filter SuiteOnly): 32
   - Failed (Filter Baseline  : 7
   - Failed (Filter CI Flakes): 2
   - Status After Filters     : failed

 Total Tests by Certification Layer: 


 => openshift-kube-conformance: (2 failures, 2 flakes)

 --> Failed tests to Review (without flakes) - Immediate action:
<empty>

 --> Failed flake tests - Statistic from OpenShift CI
Flakes	Perc	 TestName
1	0.138%	[sig-api-machinery] CustomResourcePublishOpenAPI [Privileged:ClusterAdmin] works for multiple CRDs of same group and version but different kinds [Conformance] [Suite:openshift/conformance/parallel/minimal] [Suite:k8s]
2	0.275%	[sig-api-machinery] ResourceQuota should create a ResourceQuota and capture the life of a secret. [Conformance] [Suite:openshift/conformance/parallel/minimal] [Suite:k8s]


 => openshift-conformance-validated: (7 failures, 5 flakes)

 --> Failed tests to Review (without flakes) - Immediate action:
[sig-network-edge][Feature:Idling] Unidling should handle many TCP connections by possibly dropping those over a certain bound [Serial] [Skipped:Network/OVNKubernetes] [Suite:openshift/conformance/serial]
[sig-storage] CSI Volumes [Driver: csi-hostpath] [Testpattern: Dynamic PV (default fs)] provisioning should provision storage with pvc data source [Suite:openshift/conformance/parallel] [Suite:k8s]

 --> Failed flake tests - Statistic from OpenShift CI
Flakes	Perc	 TestName
101	10.576%	[sig-arch][bz-DNS][Late] Alerts alert/KubePodNotReady should not be at or above pending in ns/openshift-dns [Suite:openshift/conformance/parallel]
67	7.016%	[sig-arch][bz-Routing][Late] Alerts alert/KubePodNotReady should not be at or above pending in ns/openshift-ingress [Suite:openshift/conformance/parallel]
2	0.386%	[sig-imageregistry] Image registry should redirect on blob pull [Suite:openshift/conformance/parallel]
32	4.848%	[sig-network][Feature:EgressFirewall] egressFirewall should have no impact outside its namespace [Suite:openshift/conformance/parallel]
11	2.402%	[sig-network][Feature:EgressFirewall] when using openshift-sdn should ensure egressnetworkpolicy is created [Suite:openshift/conformance/parallel]

 Data Saved to directory './processed/'
```

### Understanding the Result files

The data extracted to local storage contains the following files for each plugin:

- `test_${PLUGIN_NAME}_baseline_failures.txt`: List of test failures from the baseline execution
- `test_${PLUGIN_NAME}_provider_failures.txt`: List of test failures from the execution
- `test_${PLUGIN_NAME}_provider_filter1-suite.txt`: List of test failures included on suite
- `test_${PLUGIN_NAME}_provider_filter2-baseline.txt`: List of test failures tests* after applying all filters
- `test_${PLUGIN_NAME}_provider_suite_full.txt`: List with suite e2e tests

The base directory (`./results-provider-processed`) also contains the entire errors message for each failed tests. Those errors are saved into individual files onto those sub-directories (for each plugin):

- `failures-baseline/${PLUGIN_NAME}_${INDEX}-failure.txt`: the error summary
- `failures-baseline/${PLUGIN_NAME}_${INDEX}-systemOut.txt`: the entire stdout of the failed plugin

Considerations:

- `${PLUGIN_NAME}`: currently these plugins names are valid: [`openshift-validated`, `kubernetes-conformance`]
- `${INDEX}` is the simple index ordered by test name on the list

Example of files on the extracted directory:

```
$ tree processed/
processed/
├── failures-baseline
[redacted]
├── failures-provider
[redacted]
├── failures-provider-filtered
│   ├── kubernetes-conformance_1-1-failure.txt
│   ├── kubernetes-conformance_1-1-systemOut.txt
│   ├── kubernetes-conformance_2-2-failure.txt
│   ├── kubernetes-conformance_2-2-systemOut.txt
│   ├── openshift-validated_1-31-failure.txt
│   ├── openshift-validated_1-31-systemOut.txt
[redacted]
│   ├── openshift-validated_7-1-failure.txt
│   └── openshift-validated_7-1-systemOut.txt
├── tests_kubernetes-conformance_baseline_failures.txt
├── tests_kubernetes-conformance_provider_failures.txt
├── tests_kubernetes-conformance_provider_filter1-suite.txt
├── tests_kubernetes-conformance_provider_filter2-baseline.txt
├── tests_kubernetes-conformance_suite_full.txt
├── tests_openshift-validated_baseline_failures.txt
├── tests_openshift-validated_provider_failures.txt
├── tests_openshift-validated_provider_filter1-suite.txt
├── tests_openshift-validated_provider_filter2-baseline.txt
└── tests_openshift-validated_suite_full.txt

3 directories, 300 files
```

### Review Guidelines

Overview of the steps:

Items to review:

- OCP version match the certification request
- Review the result file
- Check if the failures are 0, if not, need to check one by one
- Check details of each test failed on the sub-directory `failures-provider-filtered`
