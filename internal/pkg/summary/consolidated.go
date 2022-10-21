package summary

import (
	"bufio"
	"fmt"
	"os"
	"sort"

	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"

	"github.com/xuri/excelize/v2"
	"github.com/redhat-openshift-ecosystem/provider-certification-tool/internal/pkg/sippy"
)

// ConsolidatedSummary Aggregate the results of provider and baseline
type ConsolidatedSummary struct {
	Provider *ResultSummary
	Baseline *ResultSummary
	Suites   *OpenshiftTestsSuites
}

func (cs *ConsolidatedSummary) Process() error {

	// Load Result Summary from Archives
	err := cs.Provider.Populate()
	if err != nil {
		fmt.Println("ERROR processing provider results...")
		return err
	}

	err = cs.Baseline.Populate()
	if err != nil {
		fmt.Println("ERROR processing baseline results...")
		return err
	}

	// Load Tests for each suites
	err = cs.Suites.LoadAll()
	if err != nil {
		return err
	}

	// Apply filters
	err = cs.applyFilterSuite()
	if err != nil {
		return err
	}

	err = cs.applyFilterBaseline()
	if err != nil {
		return err
	}

	err = cs.applyFilterFlaky()
	if err != nil {
		return err
	}

	return nil
}

// applyFilterSuite process the FailedList for each plugin, getting **intersection** tests
// for respective suite.
func (cs *ConsolidatedSummary) applyFilterSuite() error {
	err := cs.applyFilterSuiteForPlugin("kubernetes-conformance")
	if err != nil {
		return err
	}

	err = cs.applyFilterSuiteForPlugin("openshift-validated")
	if err != nil {
		return err
	}

	return nil
}

// applyFilterSuiteForPlugin calculates the intersection of Provider Failed AND suite
func (cs *ConsolidatedSummary) applyFilterSuiteForPlugin(plugin string) error {
	var e2eSuite []string
	var e2eFailures []string
	var e2eFailuresFiltered []string
	hashSuite := make(map[string]struct{})

	switch plugin {
	case "kubernetes-conformance":
		e2eSuite = cs.Suites.KubernetesConformance.Tests
		e2eFailures = cs.Provider.Openshift.PluginResultK8sConformance.FailedList
	case "openshift-validated":
		e2eSuite = cs.Suites.OpenshiftConformance.Tests
		e2eFailures = cs.Provider.Openshift.PluginResultOCPValidated.FailedList
	default:
		fmt.Println("Suite not found!\n")
	}

	for _, v := range e2eSuite {
		hashSuite[v] = struct{}{}
	}

	for _, v := range e2eFailures {
		if _, ok := hashSuite[v]; ok {
			e2eFailuresFiltered = append(e2eFailuresFiltered, v)
		}
	}
	sort.Strings(e2eFailuresFiltered)

	switch plugin {
	case "kubernetes-conformance":
		cs.Provider.Openshift.PluginResultK8sConformance.FailedFilterSuite = e2eFailuresFiltered
	case "openshift-validated":
		cs.Provider.Openshift.PluginResultOCPValidated.FailedFilterSuite = e2eFailuresFiltered
	default:
		fmt.Println("Suite not found!\n")
	}

	return nil
}

// applyFilterBaseline process the FailedFilterSuite for each plugin, **excluding** failures from
// baseline test.
func (cs *ConsolidatedSummary) applyFilterBaseline() error {
	err := cs.applyFilterBaselineForPlugin("kubernetes-conformance")
	if err != nil {
		return err
	}

	err = cs.applyFilterBaselineForPlugin("openshift-validated")
	if err != nil {
		return err
	}

	return nil
}

// applyFilterBaselineForPlugin calculates the **exclusion** tests of
// Provider Failed included on suite and Baseline failed tests.
func (cs *ConsolidatedSummary) applyFilterBaselineForPlugin(plugin string) error {
	var e2eFailuresProvider []string
	var e2eFailuresBaseline []string
	var e2eFailuresFiltered []string
	hashBaseline := make(map[string]struct{})

	switch plugin {
	case "kubernetes-conformance":
		e2eFailuresProvider = cs.Provider.Openshift.PluginResultK8sConformance.FailedFilterSuite
		e2eFailuresBaseline = cs.Baseline.Openshift.PluginResultK8sConformance.FailedList
	case "openshift-validated":
		e2eFailuresProvider = cs.Provider.Openshift.PluginResultOCPValidated.FailedFilterSuite
		e2eFailuresBaseline = cs.Baseline.Openshift.PluginResultOCPValidated.FailedList
	default:
		return errors.New("Unable to get current failures: Suite not found to apply filter: Baseline")
	}

	for _, v := range e2eFailuresBaseline {
		hashBaseline[v] = struct{}{}
	}

	for _, v := range e2eFailuresProvider {
		if _, ok := hashBaseline[v]; !ok {
			e2eFailuresFiltered = append(e2eFailuresFiltered, v)
		}
	}
	sort.Strings(e2eFailuresFiltered)

	switch plugin {
	case "kubernetes-conformance":
		cs.Provider.Openshift.PluginResultK8sConformance.FailedFilterBaseline = e2eFailuresFiltered
	case "openshift-validated":
		cs.Provider.Openshift.PluginResultOCPValidated.FailedFilterBaseline = e2eFailuresFiltered
	default:
		return errors.New("Unable to save filetered failures: Suite not found to apply filter: Baseline")
	}

	return nil
}

// applyFilterFlaky process the FailedFilterSuite for each plugin, **excluding** failures from
// baseline test.
func (cs *ConsolidatedSummary) applyFilterFlaky() error {
	err := cs.applyFilterFlakyForPlugin("kubernetes-conformance")
	if err != nil {
		return err
	}

	err = cs.applyFilterFlakyForPlugin("openshift-validated")
	if err != nil {
		return err
	}

	return nil
}

// applyFilterFlakyForPlugin query the Sippy API looking for each failed test
// on each plugin/suite, saving the list on the ResultSummary.
func (cs *ConsolidatedSummary) applyFilterFlakyForPlugin(plugin string) error {

	var ps *OPCTPluginSummary

	switch plugin {
	case "kubernetes-conformance":
		ps = cs.Provider.Openshift.PluginResultK8sConformance
	case "openshift-validated":
		ps = cs.Provider.Openshift.PluginResultOCPValidated
	default:
		return errors.New("Suite not found to apply filter: Flaky")
	}

	// TODO: define if we will check for flakes for all failures or only filtered
	// Query Flaky only the FilteredBaseline to avoid many external queries.
	api := sippy.NewSippyAPI()
	for _, name := range ps.FailedFilterBaseline {

		resp, err := api.QueryTests(&sippy.SippyTestsRequestInput{TestName: name})
		if err != nil {
			log.Errorf("#> Error querying to Sippy API: %v", err)
			continue
		}
		for _, r := range *resp {
			if _, ok := ps.FailedItems[name]; ok {
				ps.FailedItems[name].Flaky = &r
			} else {
				ps.FailedItems[name] = &PluginFailedItem{
					Name:  name,
					Flaky: &r,
				}
			}

			// Remove all flakes, regardless the percentage.
			// TODO: Review checking flaky severity
			if ps.FailedItems[name].Flaky.CurrentFlakes == 0 {
				ps.FailedFilterFlaky = append(ps.FailedFilterFlaky, name)
			}
		}
	}

	sort.Strings(ps.FailedFilterFlaky)
	return nil
}

func (cs *ConsolidatedSummary) saveResultsPlugin(path, plugin string) error {

	var resultsProvider *OPCTPluginSummary
	var resultsBaseline *OPCTPluginSummary
	var suite *OpenshiftTestsSuite
	var prefix = "tests"

	switch plugin {
	case "kubernetes-conformance":
		resultsProvider = cs.Provider.Openshift.GetResultK8SValidated()
		resultsBaseline = cs.Baseline.Openshift.GetResultK8SValidated()
		suite = cs.Suites.KubernetesConformance
	case "openshift-validated":
		resultsProvider = cs.Provider.Openshift.GetResultOCPValidated()
		resultsBaseline = cs.Baseline.Openshift.GetResultOCPValidated()
		suite = cs.Suites.OpenshiftConformance
	}

	// Save Provider failures
	filename := fmt.Sprintf("%s/%s_%s_provider_failures-1-ini.txt", path, prefix, plugin)
	if err := writeFileTestList(filename, resultsProvider.FailedList); err != nil {
		return err
	}

	// Save Provider failures with filter: Suite (only)
	filename = fmt.Sprintf("%s/%s_%s_provider_failures-2-filter1_suite.txt", path, prefix, plugin)
	if err := writeFileTestList(filename, resultsProvider.FailedFilterSuite); err != nil {
		return err
	}

	// Save Provider failures with filter: Baseline exclusion
	filename = fmt.Sprintf("%s/%s_%s_provider_failures-3-filter2_baseline.txt", path, prefix, plugin)
	if err := writeFileTestList(filename, resultsProvider.FailedFilterBaseline); err != nil {
		return err
	}

	// Save Provider failures with filter: Flaky
	filename = fmt.Sprintf("%s/%s_%s_provider_failures-4-filter3_without_flakes.txt", path, prefix, plugin)
	if err := writeFileTestList(filename, resultsProvider.FailedFilterFlaky); err != nil {
		return err
	}

	// Save the Providers failures for the latest filter to review (focus on this)
	filename = fmt.Sprintf("%s/%s_%s_provider_failures.txt", path, prefix, plugin)
	if err := writeFileTestList(filename, resultsProvider.FailedFilterBaseline); err != nil {
		return err
	}

	// Save baseline failures
	filename = fmt.Sprintf("%s/%s_%s_baseline_failures.txt", path, prefix, plugin)
	if err := writeFileTestList(filename, resultsBaseline.FailedList); err != nil {
		return err
	}

	// Save the openshift-tests suite use by this plugin:
	filename = fmt.Sprintf("%s/%s_%s_suite_full.txt", path, prefix, plugin)
	if err := writeFileTestList(filename, suite.Tests); err != nil {
		return err
	}

	return nil
}

func (cs *ConsolidatedSummary) extractFailuresDetailsByPlugin(path, plugin string) error {

	var resultsProvider *OPCTPluginSummary
	var resultsBaseline *OPCTPluginSummary
	ignoreExistingDir := true

	switch plugin {
	case "kubernetes-conformance":
		resultsProvider = cs.Provider.Openshift.GetResultK8SValidated()
		resultsBaseline = cs.Baseline.Openshift.GetResultK8SValidated()
	case "openshift-validated":
		resultsProvider = cs.Provider.Openshift.GetResultOCPValidated()
		resultsBaseline = cs.Baseline.Openshift.GetResultOCPValidated()
	}

	currentDirectory := "failures-provider-filtered"
	subdir := fmt.Sprintf("%s/%s", path, currentDirectory)
	if err := createDir(subdir, ignoreExistingDir); err != nil {
		return err
	}

	subPrefix := fmt.Sprintf("%s/%s", subdir, plugin)
	errItems := resultsProvider.FailedItems
	errList := resultsProvider.FailedFilterBaseline
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	currentDirectory = "failures-provider"
	subdir = fmt.Sprintf("%s/%s", path, currentDirectory)
	if err := createDir(subdir, ignoreExistingDir); err != nil {
		return err
	}

	subPrefix = fmt.Sprintf("%s/%s", subdir, plugin)
	errItems = resultsProvider.FailedItems
	errList = resultsProvider.FailedList
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	currentDirectory = "failures-baseline"
	subdir = fmt.Sprintf("%s/%s", path, currentDirectory)
	if err := createDir(subdir, ignoreExistingDir); err != nil {
		return err
	}

	subPrefix = fmt.Sprintf("%s/%s", subdir, plugin)
	errItems = resultsBaseline.FailedItems
	errList = resultsBaseline.FailedList
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	return nil
}

func (cs *ConsolidatedSummary) saveFailuresIndexToSheet(path string) error {

	var rowN int64
	var suite string
	var errList []string

	sheet := excelize.NewFile()
	sheetFile := fmt.Sprintf("%s/failures-index.xlsx", path)
	defer saveSheet(sheet, sheetFile)

	sheetName := "failures-provider-filtered"
	sheet.SetActiveSheet(sheet.NewSheet(sheetName))
	if err := createSheet(sheet, sheetName); err != nil {
		log.Error(err)
	} else {
		suite = "kubernetes-conformance"
		errList = cs.Provider.Openshift.GetResultK8SValidated().FailedFilterBaseline
		rowN = 2
		if err := populateSheet(sheet, sheetName, suite, errList, &rowN); err != nil {
			log.Error(err)
		}
		suite = "openshift-validated"
		errList = cs.Provider.Openshift.GetResultOCPValidated().FailedFilterBaseline
		if err := populateSheet(sheet, sheetName, suite, errList, &rowN); err != nil {
			log.Error(err)
		}
	}

	sheetName = "failures-provider"
	sheet.SetActiveSheet(sheet.NewSheet(sheetName))
	if err := createSheet(sheet, sheetName); err != nil {
		log.Error(err)
	} else {
		suite = "kubernetes-conformance"
		errList = cs.Provider.Openshift.GetResultK8SValidated().FailedList
		rowN = 2
		if err := populateSheet(sheet, sheetName, suite, errList, &rowN); err != nil {
			log.Error(err)
		}
		suite = "openshift-validated"
		errList = cs.Provider.Openshift.GetResultOCPValidated().FailedList
		if err := populateSheet(sheet, sheetName, suite, errList, &rowN); err != nil {
			log.Error(err)
		}
	}

	sheetName = "failures-baseline"
	sheet.SetActiveSheet(sheet.NewSheet(sheetName))
	if err := createSheet(sheet, sheetName); err != nil {
		log.Error(err)
	} else {
		suite = "kubernetes-conformance"
		errList = cs.Baseline.Openshift.GetResultK8SValidated().FailedList
		rowN = 2
		if err := populateSheet(sheet, sheetName, suite, errList, &rowN); err != nil {
			log.Error(err)
		}
		suite = "openshift-validated"
		errList = cs.Baseline.Openshift.GetResultOCPValidated().FailedList
		if err := populateSheet(sheet, sheetName, suite, errList, &rowN); err != nil {
			log.Error(err)
		}
	}
	return nil
}

// SaveResults dump all the results and processed to the disk to be used
// on the review process.
func (cs *ConsolidatedSummary) SaveResults(path string) error {

	if err := createDir(path, false); err != nil {
		return err
	}

	// Save the list of failures into individual files by Plugin
	if err := cs.saveResultsPlugin(path, "kubernetes-conformance"); err != nil {
		return err
	}
	if err := cs.saveResultsPlugin(path, "openshift-validated"); err != nil {
		return err
	}

	// Extract errors details to sub directories
	if err := cs.extractFailuresDetailsByPlugin(path, "kubernetes-conformance"); err != nil {
		return err
	}
	if err := cs.extractFailuresDetailsByPlugin(path, "openshift-validated"); err != nil {
		return err
	}

	// Save one Sheet file with Failures to be used on the review process
	if err := cs.saveFailuresIndexToSheet(path); err != nil {
		return err
	}

	fmt.Printf("\n Data Saved to directory '%s/'\n", path)
	return nil
}

// writeFileTestList saves the list of test names to a new text file
func writeFileTestList(filename string, data []string) error {
	fd, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer fd.Close()

	if err != nil {
		log.Fatalf("failed creating file: %s", err)
	}

	writer := bufio.NewWriter(fd)
	defer writer.Flush()

	for _, line := range data {
		_, err = writer.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

// extractTestErrors dumps the test error, summary and stdout, to be saved
// to individual files.
func extractTestErrors(prefix string, items map[string]*PluginFailedItem, failures []string) error {
	for idx, line := range failures {
		if _, ok := items[line]; ok {
			file := fmt.Sprintf("%s_%d-failure.txt", prefix, idx+1)
			err := writeErrorToFile(file, items[line].Failure)
			if err != nil {
				log.Errorf("Error writing Failure for test: %s\n", line)
			}

			file = fmt.Sprintf("%s_%d-systemOut.txt", prefix, idx+1)
			err = writeErrorToFile(file, items[line].SystemOut)
			if err != nil {
				log.Errorf("Error writing SystemOut for test: %s\n", line)
			}
		}
	}
	return nil
}

// writeErrorToFile save the entire buffer to individual file.
func writeErrorToFile(file, data string) error {
	fd, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer fd.Close()

	if err != nil {
		log.Fatalf("failed creating file: %s", err)
	}

	writer := bufio.NewWriter(fd)
	defer writer.Flush()

	_, err = writer.WriteString(data)
	if err != nil {
		return err
	}

	return nil
}

// createDir checks if the directory exists, if not creates it, otherwise log and return error
func createDir(path string, ignoreexisting bool) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		if ignoreexisting == true {
			return nil
		}
		log.Errorf("ERROR: Directory already exists [%s]: %v", path, err)
		return err
	}

	if err := os.Mkdir(path, os.ModePerm); err != nil {
		log.Errorf("ERROR: Unable to create directory [%s]: %v", path, err)
		return err
	}
	return nil
}

// createSheet creates the excel spreadsheet headers
func createSheet(sheet *excelize.File, sheeName string) error {
	header := map[string]string{
		"A1": "Plugin", "B1": "Index", "C1": "Error_Directory",
		"D1": "Test_Name", "E1": "Notes_Review", "F1": "References"}

	// create header
	for k, v := range header {
		sheet.SetCellValue(sheeName, k, v)
	}

	return nil
}

// populateGsheet fill each row per error item.
func populateSheet(sheet *excelize.File, sheeName, suite string, list []string, rowN *int64) error {

	for idx, v := range list {
		sheet.SetCellValue(sheeName, fmt.Sprintf("A%d", *rowN), suite)
		sheet.SetCellValue(sheeName, fmt.Sprintf("B%d", *rowN), idx+1)
		sheet.SetCellValue(sheeName, fmt.Sprintf("C%d", *rowN), sheeName)
		sheet.SetCellValue(sheeName, fmt.Sprintf("D%d", *rowN), v)
		sheet.SetCellValue(sheeName, fmt.Sprintf("E%d", *rowN), "TODO Review")
		sheet.SetCellValue(sheeName, fmt.Sprintf("F%d", *rowN), "")
		*(rowN) += 1
	}

	return nil
}

// save the excel sheet to the disk.
func saveSheet(sheet *excelize.File, sheetFileName string) {
	if err := sheet.SaveAs(sheetFileName); err != nil {
		log.Error(err)
	}
}
