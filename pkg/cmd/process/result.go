package process

import (
	"compress/gzip"
	"fmt"
	"os"

	"github.com/pkg/errors"

	"github.com/vmware-tanzu/sonobuoy/pkg/client/results"
	"github.com/vmware-tanzu/sonobuoy/pkg/discovery"
)

const (
	// OpenShift Custom Resources
	openshiftCrInfrastructureFilePath = "resources/cluster/config.openshift.io_v1_infrastructures.json"
	openshiftCrCvoPath                = "resources/cluster/config.openshift.io_v1_clusterversions.json"
	openshiftCrCoPath                 = "resources/cluster/config.openshift.io_v1_clusteroperators.json"
)

// ResultSummary holds the summary of a single execution
type ResultSummary struct {
	name      string
	archive   string
	cluster   discovery.ClusterSummary
	openshift *OpenShiftSummary
}


func getPluginList(r *results.Reader) ([]string, error) {
	runInfo := discovery.RunInfo{}
	err := r.WalkFiles(func(path string, info os.FileInfo, err error) error {
		return results.ExtractFileIntoStruct(r.RunInfoFile(), path, info, &runInfo)
	})

	return runInfo.LoadedPlugins, errors.Wrap(err, "finding plugin list")
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
