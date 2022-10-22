package summary

import (
	"compress/gzip"
	"fmt"
	"os"

	"github.com/pkg/errors"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/vmware-tanzu/sonobuoy/pkg/client/results"
	"github.com/vmware-tanzu/sonobuoy/pkg/discovery"
)

const (
	// OpenShift Custom Resources locations on archive file
	resourceInfrastructuresPath  = "resources/cluster/config.openshift.io_v1_infrastructures.json"
	resourceClusterVersionsPath  = "resources/cluster/config.openshift.io_v1_clusterversions.json"
	resourceClusterOperatorsPath = "resources/cluster/config.openshift.io_v1_clusteroperators.json"
)

// ResultSummary holds the summary of a single execution
type ResultSummary struct {
	Name      string
	Archive   string
	Sonobuoy  *SonobuoySummary
	OpenShift *OpenShiftSummary
	reader    *results.Reader
}

// Populate eentry point to process the results into the summary structure.
func (rs *ResultSummary) Populate() error {

	cleanup, err := rs.openReader()
	defer cleanup()
	if err != nil {
		return err
	}

	// Report on all plugins or the specified one.
	plugins, err := rs.getPluginList()
	if err != nil {
		return errors.Wrapf(err, "unable to determine plugins to report on")
	}
	if len(plugins) == 0 {
		return fmt.Errorf("no plugins specified by either the --plugin flag or tarball metadata")
	}

	var lastErr error
	for _, plugin := range plugins {
		err := rs.processPlugin(plugin)
		if err != nil {
			lastErr = err
		}
	}

	// TODO: review the fd usage for tarbal and file
	cleanup, err = rs.openReader()
	defer cleanup()
	if err != nil {
		return err
	}

	err = rs.populateSummary()
	if err != nil {
		lastErr = err
	}

	return lastErr
}

func (rs *ResultSummary) GetOpenShift() *OpenShiftSummary {
	return rs.OpenShift
}

func (rs *ResultSummary) GetSonobuoy() *SonobuoySummary {
	return rs.Sonobuoy
}

func (rs *ResultSummary) GetSonobuoyCluster() *discovery.ClusterSummary {
	return rs.Sonobuoy.Cluster
}

// getPluginList extract the plugin list from the archive reader.
func (rs *ResultSummary) getPluginList() ([]string, error) {
	runInfo := discovery.RunInfo{}
	err := rs.reader.WalkFiles(func(path string, info os.FileInfo, err error) error {
		return results.ExtractFileIntoStruct(rs.reader.RunInfoFile(), path, info, &runInfo)
	})

	return runInfo.LoadedPlugins, errors.Wrap(err, "finding plugin list")
}

// openReader returns a *results.Reader along with a cleanup function to close the
// underlying readers. The cleanup function is guaranteed to never be nil.
func (rs *ResultSummary) openReader() (func(), error) {

	filepath := rs.Archive
	fi, err := os.Stat(filepath)
	if err != nil {
		rs.reader = nil
		return func() {}, err
	}
	// When results is a directory
	if fi.IsDir() {
		rs.reader = results.NewReaderFromDir(filepath)
		return func() {}, nil
	}
	f, err := os.Open(filepath)
	if err != nil {
		rs.reader = nil
		return func() {}, errors.Wrapf(err, "could not open sonobuoy archive: %v", filepath)
	}

	gzr, err := gzip.NewReader(f)
	if err != nil {
		rs.reader = nil
		return func() { f.Close() }, errors.Wrap(err, "could not make a gzip reader")
	}

	rs.reader = results.NewReaderWithVersion(gzr, results.VersionTen)
	return func() { gzr.Close(); f.Close() }, nil
}

// processPlugin receives the plugin name and load the result file to be processed.
func (rs *ResultSummary) processPlugin(plugin string) error {

	// TODO: review the fd usage for tarbal and file
	cleanup, err := rs.openReader()
	defer cleanup()
	if err != nil {
		return err
	}

	obj, err := rs.reader.PluginResultsItem(plugin)
	if err != nil {
		return err
	}

	err = rs.processPluginResult(obj)
	if err != nil {
		return err
	}
	return nil
}

// processPluginResult receives the plugin results object and parse it to the summary.
func (rs *ResultSummary) processPluginResult(obj *results.Item) error {
	statusCounts := map[string]int{}
	var failures []results.Item
	var failedList []string

	statusCounts, failures = walkForSummary(obj, statusCounts, failures)

	total := 0
	for _, v := range statusCounts {
		total += v
	}

	failedItems := make(map[string]*PluginFailedItem, len(failures))
	for _, item := range failures {
		failedItems[item.Name] = &PluginFailedItem{
			Name: item.Name,
		}
		if _, ok := item.Details["failure"]; ok {
			failedItems[item.Name].Failure = item.Details["failure"].(string)
		}
		if _, ok := item.Details["system-out"]; ok {
			failedItems[item.Name].SystemOut = item.Details["system-out"].(string)
		}
		if _, ok := item.Details["offset"]; ok {
			failedItems[item.Name].Offset = item.Details["offset"].(int)
		}
		failedList = append(failedList, item.Name)
	}

	if err := rs.GetOpenShift().SetPluginResult(&OPCTPluginSummary{
		Name:        obj.Name,
		Status:      obj.Status,
		Total:       int64(total),
		Passed:      int64(statusCounts[results.StatusPassed]),
		Failed:      int64(statusCounts[results.StatusFailed] + statusCounts[results.StatusTimeout]),
		Timeout:     int64(statusCounts[results.StatusTimeout]),
		Skipped:     int64(statusCounts[results.StatusSkipped]),
		FailedList:  failedList,
		FailedItems: failedItems,
	}); err != nil {
		return err
	}

	delete(statusCounts, results.StatusPassed)
	delete(statusCounts, results.StatusFailed)
	delete(statusCounts, results.StatusTimeout)
	delete(statusCounts, results.StatusSkipped)

	return nil
}

// populateSummary load all files from archive reader and extract desired
// information to the ResultSummary.
func (rs *ResultSummary) populateSummary() error {

	sbCluster := discovery.ClusterSummary{}
	ocpInfra := configv1.InfrastructureList{}
	ocpCV := configv1.ClusterVersionList{}
	ocpCO := configv1.ClusterOperatorList{}

	// For summary and dump views, get the item as an object to iterate over.
	err := rs.reader.WalkFiles(func(path string, info os.FileInfo, err error) error {

		err = results.ExtractFileIntoStruct(results.ClusterHealthFilePath(), path, info, &sbCluster)
		if err != nil {
			return err
		}
		err = results.ExtractFileIntoStruct(resourceInfrastructuresPath, path, info, &ocpInfra)
		if err != nil {
			return err
		}
		err = results.ExtractFileIntoStruct(resourceClusterVersionsPath, path, info, &ocpCV)
		if err != nil {
			return err
		}
		err = results.ExtractFileIntoStruct(resourceClusterOperatorsPath, path, info, &ocpCO)
		if err != nil {
			return err
		}
		return err
	})
	if err != nil {
		return err
	}

	if err := rs.GetSonobuoy().SetCluster(&sbCluster); err != nil {
		return err
	}
	if err := rs.GetOpenShift().SetInfrastructure(&ocpInfra); err != nil {
		return err
	}
	if err := rs.GetOpenShift().SetClusterVersion(&ocpCV); err != nil {
		return err
	}
	if err := rs.GetOpenShift().SetClusterOperator(&ocpCO); err != nil {
		return err
	}

	return nil
}

// walkForSummary recursively walk through the result YAML file extracting the counters
// and failures.
func walkForSummary(result *results.Item, statusCounts map[string]int, failList []results.Item) (map[string]int, []results.Item) {
	if len(result.Items) > 0 {
		for _, item := range result.Items {
			statusCounts, failList = walkForSummary(&item, statusCounts, failList)
		}
		return statusCounts, failList
	}

	statusCounts[result.Status]++

	if result.Status == results.StatusFailed || result.Status == results.StatusTimeout {
		result.Details["offset"] = statusCounts[result.Status]
		failList = append(failList, *result)
	}

	return statusCounts, failList
}
