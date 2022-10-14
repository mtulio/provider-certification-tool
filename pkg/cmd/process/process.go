package process

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

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

type summaryInput struct {
	name      string
	archive   string
	cluster   discovery.ClusterSummary
	openshift *OpenShiftSummary
	input     *resultsInput
}

type resultsSummary struct {
	provider *summaryInput
	baseline *summaryInput
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

	results := resultsSummary{
		provider: &summaryInput{
			name:      "provider",
			archive:   input.archive,
			input:     &input,
			openshift: NewOpenShiftSummary(),
		},
		baseline: &summaryInput{
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

	err := populateResult(results.provider)
	if err != nil {
		fmt.Println("ERROR processing provider results...")
		return err
	}
	if results.baseline.archive != "" {
		err := populateResult(results.baseline)
		if err != nil {
			fmt.Println("ERROR processing baseline results...")
			return err
		}
	}

	// Read Suites
	err = results.suites.LoadAll()
	if err != nil {
		return err
	}
	fmt.Printf("OCP: %d\n", results.suites.openshiftConformance.count)
	fmt.Printf("k8s: %d\n", results.suites.kubernetesConformance.count)

	err = printAggregatedTable(&results)
	if err != nil {
		return err
	}

	return err
}

func populateResult(result *summaryInput) error {
	input := *result.input
	r, cleanup, err := getReader(result.archive)
	defer cleanup()
	if err != nil {
		return err
	}

	// Report on all plugins or the specified one.
	plugins, err := getPluginList(r)
	if err != nil {
		return errors.Wrapf(err, "unable to determine plugins to report on")
	}
	if len(plugins) == 0 {
		return fmt.Errorf("no plugins specified by either the --plugin flag or tarball metadata")
	}

	var lastErr error
	for i, plugin := range plugins {
		input.plugin = plugin

		// Load file with a new reader since we can't assume this reader has rewind
		// capabilities.
		r, cleanup, err = getReader(result.archive)
		defer cleanup()
		if err != nil {
			lastErr = err
		}

		err = printSinglePlugin(input, r, result)
		if err != nil {
			lastErr = err
		}

		// Seperator line, but don't print a needless one at the end.
		if i+1 < len(plugins) {
			fmt.Println()
		}
	}

	input.plugin = clusterHealthSummaryPluginName
	r, cleanup, err = getReader(result.archive)
	defer cleanup()
	if err != nil {
		lastErr = err
	}

	err = populateSummary(r, result)
	if err != nil {
		lastErr = err
	}

	return lastErr
}

// printHealthSummary pretends to work like printSinglePlugin
// but for a "fake" plugin that prints health information
func populateSummary(r *results.Reader, summary *summaryInput) error {

	var err error

	// For summary and dump views, get the item as an object to iterate over.
	ocpInfra := OpenShiftCrInfrastructures{}
	ocpCVO := OpenShiftCrCvo{}
	ocpCO := OpenShiftCrCo{}

	err = r.WalkFiles(func(path string, info os.FileInfo, err error) error {
		err = results.ExtractFileIntoStruct(results.ClusterHealthFilePath(), path, info, &summary.cluster)
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
	summary.openshift.setFromInfraCR(&ocpInfra)
	summary.openshift.setFromCvoCR(&ocpCVO)
	summary.openshift.setFromCoCR(&ocpCO)

	return nil
}

func printAggregatedTable(results *resultsSummary) error {
	fmt.Printf("\n> OpenShift Provider Certification Execution Summary <\n\n")

	pOCP := results.provider.openshift
	pCL := results.provider.cluster

	bOCP := results.baseline.openshift
	bCL := results.baseline.cluster

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

type humanReadableWriter struct {
	w io.Writer
}

func (hw *humanReadableWriter) Write(b []byte) (int, error) {
	newb := bytes.Replace(b, []byte(`\n`), []byte("\n"), -1)
	newb = bytes.Replace(newb, []byte(`\t`), []byte("\t"), -1)
	_, err := hw.w.Write(newb)
	return len(b), err
}

func printSinglePlugin(input resultsInput, r *results.Reader, summary *summaryInput) error {
	// If we want to dump the whole file, don't decode to an Item object first.
	if input.mode == resultModeDump {
		fReader, err := r.PluginResultsReader(input.plugin)
		if err != nil {
			return errors.Wrapf(err, "failed to get results reader for plugin %v", input.plugin)
		}
		_, err = io.Copy(os.Stdout, fReader)
		return err
	} else if input.mode == resultModeReadable {
		fReader, err := r.PluginResultsReader(input.plugin)
		if err != nil {
			return errors.Wrapf(err, "failed to get results reader for plugin %v", input.plugin)
		}
		writer := &humanReadableWriter{os.Stdout}
		_, err = io.Copy(writer, fReader)
		if err != nil {
			return errors.Wrapf(err, "failed to copy data for plugin %v", input.plugin)
		}
		return err
	}

	// For summary and detailed views, get the item as an object to iterate over.
	obj, err := r.PluginResultsItem(input.plugin)
	if err != nil {
		return err
	}

	obj = obj.GetSubTreeByName(input.node)
	if obj == nil {
		return fmt.Errorf("node named %q not found", input.node)
	}

	switch input.mode {
	case resultModeDetailed:
		return printResultsDetails([]string{}, obj, input)
	default:
		return printResultsSummary(obj, summary)
	}
}

func getPluginList(r *results.Reader) ([]string, error) {
	runInfo := discovery.RunInfo{}
	err := r.WalkFiles(func(path string, info os.FileInfo, err error) error {
		return results.ExtractFileIntoStruct(r.RunInfoFile(), path, info, &runInfo)
	})

	return runInfo.LoadedPlugins, errors.Wrap(err, "finding plugin list")
}

func printResultsDetails(treePath []string, o *results.Item, input resultsInput) error {
	if o == nil {
		return nil
	}

	if len(o.Items) > 0 {
		treePath = append(treePath, o.Name)
		for _, v := range o.Items {
			if err := printResultsDetails(treePath, &v, input); err != nil {
				return err
			}
		}
		return nil
	}

	leafFile := getFileFromMeta(o.Metadata)
	if leafFile == "" {
		// Print each leaf node as a json object. Add the path as a metadata field for access by the end user.
		if o.Metadata == nil {
			o.Metadata = map[string]string{}
		}
		o.Metadata["path"] = strings.Join(treePath, "|")
		b, err := json.Marshal(o)
		if err != nil {
			return errors.Wrap(err, "marshalling item to json")
		}
		fmt.Println(string(b))
	} else {
		// Load file with a new reader since we can't assume this reader has rewind
		// capabilities.
		r, cleanup, err := getReader(input.archive)
		defer cleanup()
		if err != nil {
			return errors.Wrapf(err, "reading archive to get file %v", leafFile)
		}
		resultFile := path.Join(results.PluginsDir, input.plugin, leafFile)
		filereader, err := r.FileReader(resultFile)
		if err != nil {
			return err
		}

		if input.skipPrefix {
			_, err = io.Copy(os.Stdout, filereader)
			return err
		} else {
			// When printing items like this we want the name of the node in
			// the prefix. In the "junit" version, we do not, since the name is
			// already visible on the object.
			treePath = append(treePath, o.Name)
			fmt.Printf("%v ", strings.Join(treePath, "|"))
			_, err = io.Copy(os.Stdout, filereader)
			return err
		}
	}

	return nil
}

func printResultsSummary(o *results.Item, summary *summaryInput) error {
	statusCounts := map[string]int{}
	var failedList []string

	statusCounts, failedList = walkForSummary(o, statusCounts, failedList)

	total := 0
	for _, v := range statusCounts {
		total += v
	}

	summary.openshift.setPluginResult(&OPCTPluginSummary{
		Name:       o.Name,
		Status:     o.Status,
		Total:      int64(total),
		Passed:     int64(statusCounts[results.StatusPassed]),
		Failed:     int64(statusCounts[results.StatusFailed] + statusCounts[results.StatusTimeout]),
		Timeout:    int64(statusCounts[results.StatusTimeout]),
		Skipped:    int64(statusCounts[results.StatusSkipped]),
		FailedList: failedList,
	})

	// fmt.Printf("\n-> Plugin: %s\n", o.Name)
	// fmt.Println("Status:", o.Status)
	// fmt.Println("Total:", total)

	// // We want to print the built-in status type results first before printing any custom statuses, so print first then delete.
	// fmt.Println("Passed:", statusCounts[results.StatusPassed])
	// fmt.Println("Failed:", statusCounts[results.StatusFailed]+statusCounts[results.StatusTimeout])
	// fmt.Println("Skipped:", statusCounts[results.StatusSkipped])

	delete(statusCounts, results.StatusPassed)
	delete(statusCounts, results.StatusFailed)
	delete(statusCounts, results.StatusTimeout)
	delete(statusCounts, results.StatusSkipped)

	// We want the custom statuses to always be printed in order so sort them before proceeding
	keys := []string{}
	for k := range statusCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	keys = unique(keys)

	// for _, k := range keys {
	// 	fmt.Printf("%v: %v\n", k, statusCounts[k])
	// }

	// if len(failedList) > 0 {
	// 	fmt.Print("\nFailed tests:\n")
	// 	fmt.Print(strings.Join(failedList, "\n"))
	// 	fmt.Println()
	// }

	return nil
}

func unique(original []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range original {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
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

// getFileFromMeta pulls the file out of the given metadata but also
// converts it to a slash-based-seperator since that is what is internal
// to the tar file. The metadata is written by the node and so may use
// Windows seperators.
func getFileFromMeta(m map[string]string) string {
	if m == nil {
		return ""
	}
	return toSlash(m["file"])
}

// toSlash is a (for our purpose) an improved version of filepath.ToSlash which ignores the
// current OS seperator and simply converts all windows `\` to `/`.
func toSlash(path string) string {
	return strings.ReplaceAll(path, string(windowsSeperator), "/")
}
