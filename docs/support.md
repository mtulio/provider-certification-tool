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
    --base opct_baseline-ocp_4,11.4-platform_none-hw-date_uuid.tar.gz \
    --base-suite-ocp ./test-list_openshift-tests_openshift-validated.txt \
    --base-suite-k8s ./test-list_openshift-tests_kubernetes-conformance.txt \
    202210132151_sonobuoy_6af99324-2dc6-4de4-938c-200b84111481.tar.gz
```

Compare the results AND extract the files to local directory `./results-provider-processed`

```bash
./openshift-provider-cert-linux-amd64 process \
    --base opct_baseline-ocp_4,11.4-platform_none-hw-date_uuid.tar.gz \
    --base-suite-ocp ./test-list_openshift-tests_openshift-validated.txt \
    --base-suite-k8s ./test-list_openshift-tests_kubernetes-conformance.txt \
    --save-to processed \
    202210132151_sonobuoy_6af99324-2dc6-4de4-938c-200b84111481.tar.gz
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
