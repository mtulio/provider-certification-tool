package process

import (
	"compress/gzip"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/vmware-tanzu/sonobuoy/pkg/client/results"
	"github.com/vmware-tanzu/sonobuoy/pkg/discovery"
	"github.com/vmware-tanzu/sonobuoy/pkg/errlog"
	"text/tabwriter"
)

const (
	// resultModeReport prints a human-readable summary of the results to stdout.
	resultModeReport = "report"

	// resultModeDetailed will dump each leaf node (e.g. test) as a json object. If the results
	// are just references to files (like systemd-logs) then it will print the file for each
	// leaf node, prefixed with the path.
	resultModeDetailed = "detailed"

	// resultModeDump will just copy the post-processed yaml file to stdout.
	resultModeDump = "dump"

	//resultModeReadable will copy the post-processed yaml file to stdout and replace \n and \t with new lines and tabs respectively.
	resultModeReadable = "readable"

	windowsSeperator = `\`

	//Name of the "fake" plugin used to enable printing the health summary.
	//This name needs to be reserved to avoid conflicts with a plugin with the same name
	clusterHealthSummaryPluginName = "sonobuoy"

	// OpenShift Custom Resources
	openshiftCrInfrastructureFilePath = "resources/cluster/config.openshift.io_v1_infrastructures.json"
	openshiftCrCvoPath                = "resources/cluster/config.openshift.io_v1_clusterversions.json"
	openshiftCrCoPath                 = "resources/cluster/config.openshift.io_v1_clusteroperators.json"
)

type resultsInput struct {
	archive     string
	archiveBase string
	plugin      string
	suiteOCP    string
	suiteKube   string
	mode        string
	node        string
	skipPrefix  bool
}

// ResultSummary holds the summary of a single execution
type ResultSummary struct {
	name      string
	archive   string
	cluster   discovery.ClusterSummary
	openshift *OpenShiftSummary
	input     *resultsInput
}

// ConsolidatedSummary Aggregate the results of provider and baseline
type ConsolidatedSummary struct {
	provider *ResultSummary
	baseline *ResultSummary
	suites   *openshiftTestsSuites
}

func NewCmdProcess() *cobra.Command {
	data := resultsInput{}
	cmd := &cobra.Command{
		Use:   "process archive.tar.gz",
		Short: "Inspect plugin results.",
		Run: func(cmd *cobra.Command, args []string) {
			data.archive = args[0]
			if err := processResult(data); err != nil {
				errlog.LogError(errors.Wrapf(err, "could not process archive: %v", args[0]))
				os.Exit(1)
			}
		},
		Args: cobra.ExactArgs(1),
	}

	cmd.Flags().StringVarP(
		&data.archiveBase, "base", "b", "",
		"Base result archive file. Example: -b file.tar.gz",
	)
	cmd.MarkFlagRequired("base")
	cmd.Flags().StringVarP(
		&data.suiteOCP, "base-suite-ocp", "o", "",
		"Base suite reference. Example: -b openshift-tests-openshift-conformance.txt",
	)
	cmd.MarkFlagRequired("base-suite-ocp")
	cmd.Flags().StringVarP(
		&data.suiteKube, "base-suite-k8s", "k", "",
		"Base suite reference. Example: -b openshift-tests-kube-conformance.txt",
	)
	cmd.MarkFlagRequired("base-suite-k8s")
	return cmd
}

// getReader returns a *results.Reader along with a cleanup function to close the
// underlying readers. The cleanup function is guaranteed to never be nil.
func getReader(filepath string) (*results.Reader, func(), error) {
	fi, err := os.Stat(filepath)
	if err != nil {
		return nil, func() {}, err
	}
	if fi.IsDir() {
		return results.NewReaderFromDir(filepath), func() {}, nil
	}
	f, err := os.Open(filepath)
	if err != nil {
		return nil, func() {}, errors.Wrapf(err, "could not open sonobuoy archive: %v", filepath)
	}

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, func() { f.Close() }, errors.Wrap(err, "could not make a gzip reader")
	}

	r := results.NewReaderWithVersion(gzr, results.VersionTen)
	return r, func() { gzr.Close(); f.Close() }, nil
}

func processResult(input resultsInput) error {

	cs := ConsolidatedSummary{
		provider: &ResultSummary{
			name:      "provider",
			archive:   input.archive,
			input:     &input,
			openshift: NewOpenShiftSummary(),
		},
		baseline: &ResultSummary{
			name:      "base",
			archive:   input.archiveBase,
			input:     &input,
			openshift: NewOpenShiftSummary(),
		},
		suites: &openshiftTestsSuites{
			openshiftConformance: &openshiftTestsSuite{
				name:      "openshiftConformance",
				inputFile: input.suiteOCP,
			},
			kubernetesConformance: &openshiftTestsSuite{
				name:      "kubernetesConformance",
				inputFile: input.suiteKube,
			},
		},
	}

	err := populateResult(cs.provider)
	if err != nil {
		fmt.Println("ERROR processing provider results...")
		return err
	}

	err = populateResult(cs.baseline)
	if err != nil {
		fmt.Println("ERROR processing baseline results...")
		return err
	}

	// Read Suites
	err = cs.suites.LoadAll()
	if err != nil {
		return err
	}

	err = printAggregatedTable(&cs)
	if err != nil {
		return err
	}

	// build the filters
	// Filter1: compare  failed tests with suite, getting intersection
	// Filter2: compare results from Filter1 and exclude failed tests from the Baseline
	// err = cs.ApplyFilters()

	return err
}

func populateResult(rs *ResultSummary) error {

	reader, cleanup, err := getReader(rs.archive)
	defer cleanup()
	if err != nil {
		return err
	}

	// Report on all plugins or the specified one.
	plugins, err := getPluginList(reader)
	if err != nil {
		return errors.Wrapf(err, "unable to determine plugins to report on")
	}
	if len(plugins) == 0 {
		return fmt.Errorf("no plugins specified by either the --plugin flag or tarball metadata")
	}

	var lastErr error
	for _, plugin := range plugins {
		err := processPlugin(rs, plugin)
		if err != nil {
			lastErr = err
		}
	}

	reader, cleanup, err = getReader(rs.archive)
	defer cleanup()
	if err != nil {
		lastErr = err
	}

	err = populateSummary(reader, rs)
	if err != nil {
		lastErr = err
	}

	return lastErr
}

func processPlugin(rs *ResultSummary, plugin string) error {
	reader, cleanup, err := getReader(rs.archive)
	defer cleanup()
	if err != nil {
		return err
	}

	obj, err := reader.PluginResultsItem(plugin)
	if err != nil {
		return err
	}

	err = processPluginResult(obj, rs)
	if err != nil {
		return err
	}
	return nil
}

func processPluginResult(obj *results.Item, rs *ResultSummary) error {
	statusCounts := map[string]int{}
	var failedList []string

	statusCounts, failedList = walkForSummary(obj, statusCounts, failedList)

	total := 0
	for _, v := range statusCounts {
		total += v
	}

	rs.openshift.setPluginResult(&OPCTPluginSummary{
		Name:       obj.Name,
		Status:     obj.Status,
		Total:      int64(total),
		Passed:     int64(statusCounts[results.StatusPassed]),
		Failed:     int64(statusCounts[results.StatusFailed] + statusCounts[results.StatusTimeout]),
		Timeout:    int64(statusCounts[results.StatusTimeout]),
		Skipped:    int64(statusCounts[results.StatusSkipped]),
		FailedList: failedList,
	})

	delete(statusCounts, results.StatusPassed)
	delete(statusCounts, results.StatusFailed)
	delete(statusCounts, results.StatusTimeout)
	delete(statusCounts, results.StatusSkipped)

	return nil
}

// printHealthSummary pretends to work like printSinglePlugin
// but for a "fake" plugin that prints health information
func populateSummary(r *results.Reader, rs *ResultSummary) error {

	ocpInfra := OpenShiftCrInfrastructures{}
	ocpCVO := OpenShiftCrCvo{}
	ocpCO := OpenShiftCrCo{}

	// For summary and dump views, get the item as an object to iterate over.
	err := r.WalkFiles(func(path string, info os.FileInfo, err error) error {
		err = results.ExtractFileIntoStruct(results.ClusterHealthFilePath(), path, info, &rs.cluster)
		if err != nil {
			return err
		}
		err = results.ExtractFileIntoStruct(openshiftCrInfrastructureFilePath, path, info, &ocpInfra)
		if err != nil {
			return err
		}
		err = results.ExtractFileIntoStruct(openshiftCrCvoPath, path, info, &ocpCVO)
		if err != nil {
			return err
		}
		err = results.ExtractFileIntoStruct(openshiftCrCoPath, path, info, &ocpCO)
		if err != nil {
			return err
		}
		return err
	})
	if err != nil {
		return err
	}

	rs.openshift.setFromInfraCR(&ocpInfra)
	rs.openshift.setFromCvoCR(&ocpCVO)
	rs.openshift.setFromCoCR(&ocpCO)

	return nil
}

func printAggregatedTable(cs *ConsolidatedSummary) error {
	fmt.Printf("\n> OpenShift Provider Certification Summary <\n\n")

	pOCP := cs.provider.openshift
	pCL := cs.provider.cluster

	bOCP := cs.baseline.openshift
	bCL := cs.baseline.cluster

	newLineWithTab := "\t\t\n"
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	fmt.Fprintf(tbWriter, " Kubernetes API Server version\t: %s\t: %s\n", pCL.APIVersion, bCL.APIVersion)
	fmt.Fprintf(tbWriter, " OpenShift Container Platform version\t: %s\t: %s\n", pOCP.cvoStatusDesiredVersion, bOCP.cvoStatusDesiredVersion)
	fmt.Fprintf(tbWriter, " - Cluster Update Progressing\t: %s\t: %s\n", pOCP.cvoCondProgressing, bOCP.cvoCondProgressing)
	fmt.Fprintf(tbWriter, " - Cluster Target Version\t: %s\t: %s\n", pOCP.cvoCondProgressingMessage, bOCP.cvoCondProgressingMessage)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\t\n")
	fmt.Fprintf(tbWriter, " - PlatformType\t: %s\t: %s\n", pOCP.infraPlatformType, bOCP.infraPlatformType)
	fmt.Fprintf(tbWriter, " - Name\t: %s\t: %s\n", pOCP.infraName, bOCP.infraName)
	fmt.Fprintf(tbWriter, " - Topology\t: %s\t: %s\n", pOCP.infraTopology, bOCP.infraTopology)
	fmt.Fprintf(tbWriter, " - ControlPlaneTopology\t: %s\t: %s\n", pOCP.infraControlPlaneTopology, bOCP.infraControlPlaneTopology)
	fmt.Fprintf(tbWriter, " - API Server URL\t: %s\t: %s\n", pOCP.infraAPIServerURL, bOCP.infraAPIServerURL)
	fmt.Fprintf(tbWriter, " - API Server URL (internal)\t: %s\t: %s\n", pOCP.infraAPIServerURLInternal, bOCP.infraAPIServerURLInternal)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Plugins summary by name:\t  Status [Total/Passed/Failed/Skipped]\t\n")

	plK8S := pOCP.getResultK8SValidated()
	name := plK8S.Name
	pOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	plK8S = bOCP.getResultK8SValidated()
	bOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)

	plOCP := pOCP.getResultOCPValidated()
	name = plOCP.Name
	pOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
	plOCP = bOCP.getResultOCPValidated()
	bOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Health summary:\t  [A=True/P=True/D=True]\t\n")
	fmt.Fprintf(tbWriter, " - Cluster Operators\t: [%d/%d/%d]\t: [%d/%d/%d]\n",
		pOCP.coCountAvailable, pOCP.coCountProgressing, pOCP.coCountDegraded,
		bOCP.coCountAvailable, bOCP.coCountProgressing, bOCP.coCountDegraded,
	)

	pNhMessage := fmt.Sprintf("%d/%d %s", pCL.NodeHealth.Total, pCL.NodeHealth.Total, "")
	if pCL.NodeHealth.Total != 0 {
		pNhMessage = fmt.Sprintf("%s (%d%%)", pNhMessage, 100*pCL.NodeHealth.Healthy/pCL.NodeHealth.Total)
	}

	bNhMessage := fmt.Sprintf("%d/%d %s", bCL.NodeHealth.Total, bCL.NodeHealth.Total, "")
	if bCL.NodeHealth.Total != 0 {
		bNhMessage = fmt.Sprintf("%s (%d%%)", bNhMessage, 100*bCL.NodeHealth.Healthy/bCL.NodeHealth.Total)
	}
	fmt.Fprintf(tbWriter, " - Node health\t: %s\t: %s\n", pNhMessage, bNhMessage)

	pPodsHealthMsg := ""
	bPodsHealthMsg := ""
	if len(pCL.PodHealth.Details) > 0 {
		phTotal := ""
		if pCL.PodHealth.Total != 0 {
			phTotal = fmt.Sprintf(" (%d%%)", 100*pCL.PodHealth.Healthy/pCL.PodHealth.Total)
		}
		pPodsHealthMsg = fmt.Sprintf("%d/%d %s", pCL.PodHealth.Healthy, pCL.PodHealth.Total, phTotal)
	}
	if len(bCL.PodHealth.Details) > 0 {
		phTotal := ""
		if bCL.PodHealth.Total != 0 {
			phTotal = fmt.Sprintf(" (%d%%)", 100*bCL.PodHealth.Healthy/bCL.PodHealth.Total)
		}
		bPodsHealthMsg = fmt.Sprintf("%d/%d %s", bCL.PodHealth.Healthy, bCL.PodHealth.Total, phTotal)
	}

	fmt.Fprintf(tbWriter, " - Pods health\t: %s\t: %s\n", pPodsHealthMsg, bPodsHealthMsg)
	tbWriter.Flush()

	return nil
}

func getPluginList(r *results.Reader) ([]string, error) {
	runInfo := discovery.RunInfo{}
	err := r.WalkFiles(func(path string, info os.FileInfo, err error) error {
		return results.ExtractFileIntoStruct(r.RunInfoFile(), path, info, &runInfo)
	})

	return runInfo.LoadedPlugins, errors.Wrap(err, "finding plugin list")
}

func walkForSummary(result *results.Item, statusCounts map[string]int, failList []string) (map[string]int, []string) {
	if len(result.Items) > 0 {
		for _, item := range result.Items {
			statusCounts, failList = walkForSummary(&item, statusCounts, failList)
		}
		return statusCounts, failList
	}

	statusCounts[result.Status]++

	if result.Status == results.StatusFailed || result.Status == results.StatusTimeout {
		failList = append(failList, result.Name)
	}

	return statusCounts, failList
}
