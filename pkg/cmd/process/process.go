package process

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/spf13/cobra"

	"github.com/vmware-tanzu/sonobuoy/pkg/client/results"
	"github.com/vmware-tanzu/sonobuoy/pkg/discovery"
	"github.com/vmware-tanzu/sonobuoy/pkg/errlog"
	"gopkg.in/yaml.v2"
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
	mode        string
	node        string
	skipPrefix  bool
}

type summaryInput struct {
	cluster   discovery.ClusterSummary
	openshift *OpenShiftSummary
}

type resultsSummary struct {
	provider *summaryInput
	baseline *summaryInput
}

func NewCmdProcess() *cobra.Command {
	data := resultsInput{}
	cmd := &cobra.Command{
		Use:   "process archive.tar.gz",
		Short: "Inspect plugin results.",
		Run: func(cmd *cobra.Command, args []string) {
			data.archive = args[0]
			if err := result(data); err != nil {
				errlog.LogError(errors.Wrapf(err, "could not process archive: %v", args[0]))
				os.Exit(1)
			}
		},
		Args: cobra.ExactArgs(1),
	}

	cmd.Flags().StringVarP(
		&data.plugin, "plugin", "p", "",
		"Which plugin to show results for. Defaults to printing them all.",
	)
	cmd.Flags().StringVarP(
		&data.archiveBase, "base", "b", "",
		"Base result archive file. Defaults to no use. Example: -b file.tar.gz",
	)
	cmd.Flags().StringVarP(
		&data.mode, "mode", "m", resultModeReport,
		`Modifies the format of the output. Valid options are report, detailed, readable, or dump.`,
	)
	cmd.Flags().StringVarP(
		&data.node, "node", "n", "",
		`Traverse results starting at the node with the given name. Defaults to the real root.`,
	)
	cmd.Flags().BoolVarP(
		&data.skipPrefix, "skip-prefix", "s", false,
		`When printing items linking to files, only print the file contents.`,
	)

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

// func populateResult(archive string, input *resultsInput) error {

// }

// result takes the resultsInput and tries to print the requested infromation from the archive.
// If there is an error printing any individual plugin, only the last error is printed and all plugins
// continue to be processed.
func result(input resultsInput) error {
	r, cleanup, err := getReader(input.archive)
	defer cleanup()
	if err != nil {
		return err
	}

	if input.archiveBase != "" {
		rb, cleanupBase, err := getReader(input.archiveBase)
		defer cleanupBase()
		if err != nil {
			return err
		}
		pluginsb, err := getPluginList(rb)
		if err != nil {
			return errors.Wrapf(err, "unable to determine plugins to report on")
		}
		if len(pluginsb) == 0 {
			return fmt.Errorf("no plugins specified by either the --plugin flag or tarball metadata")
		}
		fmt.Println(pluginsb)
	}

	// Report on all plugins or the specified one.
	plugins := []string{input.plugin}
	if len(input.plugin) == 0 {
		plugins, err = getPluginList(r)
		if err != nil {
			return errors.Wrapf(err, "unable to determine plugins to report on")
		}
		if len(plugins) == 0 {
			return fmt.Errorf("no plugins specified by either the --plugin flag or tarball metadata")
		}
	}

	var lastErr error
	summary := summaryInput{
		openshift: NewOpenShiftSummary(),
	}

	for i, plugin := range plugins {
		input.plugin = plugin

		// Load file with a new reader since we can't assume this reader has rewind
		// capabilities.
		r, cleanup, err = getReader(input.archive)
		defer cleanup()
		if err != nil {
			lastErr = err
		}

		err = printSinglePlugin(input, r, &summary)
		if err != nil {
			lastErr = err
		}

		// Seperator line, but don't print a needless one at the end.
		if i+1 < len(plugins) {
			fmt.Println()
		}
	}

	input.plugin = clusterHealthSummaryPluginName
	r, cleanup, err = getReader(input.archive)
	defer cleanup()
	if err != nil {
		lastErr = err
	}
	err = printHealthSummary(input, r, &summary)
	if err != nil {
		lastErr = err
	}

	return lastErr
}

// printHealthSummary pretends to work like printSinglePlugin
// but for a "fake" plugin that prints health information
func printHealthSummary(input resultsInput, r *results.Reader, summary *summaryInput) error {
	var err error

	//For detailed view we can just dump the contents of the clusterHealthSummaryPluginName file
	if input.mode == resultModeDetailed {
		reader, err := r.FileReader(results.ClusterHealthFilePath())
		if err != nil {
			return errors.Wrapf(err, "failed to get health summary results reader from file '%s'", results.ClusterHealthFilePath())
		}
		_, err = io.Copy(os.Stdout, reader)
		if err != nil {
			return errors.Wrapf(err, "failed to copy health summary results from file '%s'", results.ClusterHealthFilePath())
		}
		return nil
	}

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

	var data []byte
	switch input.mode {
	case resultModeDump:
		data, err = yaml.Marshal(summary.cluster)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case resultModeReadable:
		data, err = yaml.Marshal(summary.cluster)
		if err != nil {
			return err
		}
		str := string(data)
		str = strings.ReplaceAll(str, `\n`, "\n")
		str = strings.ReplaceAll(str, `\t`, "	")
		fmt.Println(str)
	default:
		err = printClusterHealthResultsSummary(summary)
		if err != nil {
			return err
		}
	}
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

// sortErrors takes a discovery.LogSummary, which is a map[string]map[string]int
// which is a map[errorName]map[fileName]errorCount
// and returns a map[string]i[]string that is a map[errorName] where the value
// is a list of file names ordered by the errorCount (descending)
func sortErrors(errorSummary discovery.LogSummary) map[string][]string {
	result := make(map[string][]string)
	for errorName, hitCounter := range errorSummary {
		sortedFileNamesList := make([]string, 0)
		for fileName := range hitCounter {
			sortedFileNamesList = append(sortedFileNamesList, fileName)
		}
		//Sort in descending order,
		//And use the values in hitCounter for the sorting
		isMore := func(i, j int) bool {
			valueI := hitCounter[sortedFileNamesList[i]]
			valueJ := hitCounter[sortedFileNamesList[j]]
			return valueI > valueJ
		}
		sort.Slice(sortedFileNamesList, isMore)
		result[errorName] = sortedFileNamesList
	}
	return result
}

// filterAndSortHealthInfoDetails takes a copy of a slice of HealthInfoDetails,
// discards the ones that are healthy,
// then sorts the remaining entries,
// and finally sorts them by namespace and name
func filterAndSortHealthInfoDetails(details []discovery.HealthInfoDetails) []discovery.HealthInfoDetails {
	result := make([]discovery.HealthInfoDetails, len(details))
	var idx int
	for _, detail := range details {
		if !detail.Healthy {
			result[idx] = detail
			idx++
		}
	}
	result = result[:idx]
	isLess := func(i, j int) bool {
		if result[i].Namespace == result[j].Namespace {
			return result[i].Name < result[j].Name
		} else {
			return result[i].Namespace < result[j].Namespace
		}
	}
	sort.Slice(result, isLess)
	return result
}

// printClusterHealthResultsSummary prints the summary of the "fake" plugin for health summary,
// tryingf to emulate the format of printResultsSummary
func printClusterHealthResultsSummary(summary *summaryInput) error {
	fmt.Printf("\n> OpenShift Provider Certification Execution Summary <\n\n")
	ocp := summary.openshift
	newLineWithTab := "\t\n"
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	fmt.Fprintf(tbWriter, " Kubernetes API Server version\t: %s\n", summary.cluster.APIVersion)
	fmt.Fprintf(tbWriter, " OpenShift Container Platform version\t: %s\n", summary.openshift.cvoStatusDesiredVersion)
	fmt.Fprintf(tbWriter, " - Cluster Update Progressing\t: %s \n", summary.openshift.cvoCondProgressing)
	fmt.Fprintf(tbWriter, " - Cluster Target Version\t: %s \n", summary.openshift.cvoCondProgressingMessage)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\n")
	fmt.Fprintf(tbWriter, " - PlatformType\t: %s \n", summary.openshift.infraPlatformType)
	fmt.Fprintf(tbWriter, " - Name\t: %s \n", summary.openshift.infraName)
	fmt.Fprintf(tbWriter, " - Topology\t: %s \n", summary.openshift.infraTopology)
	fmt.Fprintf(tbWriter, " - ControlPlaneTopology\t: %s \n", summary.openshift.infraControlPlaneTopology)
	fmt.Fprintf(tbWriter, " - API Server URL\t: %s \n", summary.openshift.infraAPIServerURL)
	fmt.Fprintf(tbWriter, " - API Server URL (internal)\t: %s \n", summary.openshift.infraAPIServerURLInternal)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Plugins summary by name:\t  Status [Total/Passed/Failed/Skipped] (Timeouts)\n")

	plK8S := ocp.getResultK8SValidated()
	res := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s \n", plK8S.Name, res)
	plOCP := ocp.getResultOCPValidated()
	res = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plK8S.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s \n", plOCP.Name, res)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Health summary:\t  [A=True/P=True/D=True]\n")
	fmt.Fprintf(tbWriter, " - Cluster Operators\t: [%d/%d/%d]\n", summary.openshift.coCountAvailable, summary.openshift.coCountProgressing, summary.openshift.coCountDegraded)

	nhTotal := ""
	if summary.cluster.NodeHealth.Total != 0 {
		nhTotal = fmt.Sprintf(" (%d%%)", 100*summary.cluster.NodeHealth.Healthy/summary.cluster.NodeHealth.Total)
	}
	fmt.Fprintf(tbWriter, " - Node health\t: %d/%d %s\n", summary.cluster.NodeHealth.Healthy, summary.cluster.NodeHealth.Total, nhTotal)

	if len(summary.cluster.PodHealth.Details) > 0 {
		phTotal := ""
		if summary.cluster.PodHealth.Total != 0 {
			phTotal = fmt.Sprintf(" (%d%%)", 100*summary.cluster.PodHealth.Healthy/summary.cluster.PodHealth.Total)
		}
		fmt.Fprintf(tbWriter, " - Pods health\t: %d/%d %s\n", summary.cluster.PodHealth.Healthy, summary.cluster.PodHealth.Total, phTotal)
	}

	tbWriter.Flush()

	//Details of the failed pods. Checking the slice length to avoid trusting the Total
	// if len(summary.cluster.NodeHealth.Details) > 0 && summary.cluster.NodeHealth.Healthy < summary.cluster.NodeHealth.Total {
	// 	fmt.Println("\nDetails for failed nodes:")
	// 	nodes := filterAndSortHealthInfoDetails(summary.cluster.NodeHealth.Details)
	// 	for _, node := range nodes {
	// 		fmt.Printf("%s Ready:%s: %s: %s\n", node.Name, node.Ready, node.Reason, node.Message)
	// 	}
	// 	fmt.Println()
	// }

	//It might be nice to group pods by namespace.
	//Also here, use len instead of trusting Total
	// if len(summary.cluster.PodHealth.Details) > 0 {
	// 	fmt.Printf(" - Pods health\t\t\t\t: %d/%d", summary.cluster.PodHealth.Healthy, summary.cluster.PodHealth.Total)
	// 	//Print the percentage only if Total is not 0 to avoid division by zero errors
	// 	if summary.cluster.PodHealth.Total != 0 {
	// 		fmt.Printf(" (%d%%)", 100*summary.cluster.PodHealth.Healthy/summary.cluster.PodHealth.Total)
	// 	}
	// 	fmt.Println()
	// 	if summary.cluster.PodHealth.Healthy < summary.cluster.PodHealth.Total {
	// 		fmt.Println("\nDetails for failed pods:")
	// 		pods := filterAndSortHealthInfoDetails(summary.cluster.PodHealth.Details)
	// 		//And then print them, sorted by namespace
	// 		for _, pod := range pods {
	// 			fmt.Printf("%s/%s Ready:%s: %s: %s\n", pod.Namespace, pod.Name, pod.Ready, pod.Reason, pod.Message)
	// 		}
	// 	}
	// }

	// if len(summary.cluster.ErrorInfo) > 0 {
	// 	fmt.Println("\nErrors detected in files:")
	// 	sortedFileNames := sortErrors(summary.cluster.ErrorInfo)
	// 	for errorType := range summary.cluster.ErrorInfo {
	// 		//Get the first item in the list of sorted file names and get the value for that file name
	// 		maxValue := summary.cluster.ErrorInfo[errorType][sortedFileNames[errorType][0]]
	// 		//Calculate the width of the string representation of the maxValue
	// 		maxWidth := len(fmt.Sprintf("%d", maxValue))
	// 		fmt.Printf("%s:\n", errorType)

	// 		for _, fileName := range sortedFileNames[errorType] {
	// 			fmt.Printf("%[1]*[2]d %[3]s\n", maxWidth, summary.cluster.ErrorInfo[errorType][fileName], fileName)
	// 		}
	// 	}
	// }

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

	// delete(statusCounts, results.StatusPassed)
	// delete(statusCounts, results.StatusFailed)
	// delete(statusCounts, results.StatusTimeout)
	// delete(statusCounts, results.StatusSkipped)

	// // We want the custom statuses to always be printed in order so sort them before proceeding
	// keys := []string{}
	// for k := range statusCounts {
	// 	keys = append(keys, k)
	// }
	// sort.Strings(keys)

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

// stringsToRegexp just makes a regexp out of the string array that will match any of the given values.
func stringsToRegexp(testCases []string) string {
	testNames := make([]string, len(testCases))
	for i, tc := range testCases {
		testNames[i] = regexp.QuoteMeta(tc)
	}
	return strings.Join(testNames, "|")
}

func failedTestsFromTar(tarballPath, plugin string) ([]string, error) {
	r, cleanup, err := getReader(tarballPath)
	defer cleanup()
	if err != nil {
		return nil, err
	}

	obj, err := r.PluginResultsItem(plugin)
	if err != nil {
		return nil, err
	}

	statusCounts := map[string]int{}
	var failedList []string
	_, failedList = walkForSummary(obj, statusCounts, failedList)
	return failedList, nil
}
