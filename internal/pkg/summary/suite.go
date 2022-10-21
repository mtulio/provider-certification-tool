package summary

import (
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

const (
	SuiteNameKubernetesConformance = "kubernetes/conformance"
	SuiteNameOpenshiftConformance  = "openshift/conformance"
)

type OpenshiftTestsSuites struct {
	KubernetesConformance *OpenshiftTestsSuite
	OpenshiftConformance  *OpenshiftTestsSuite
}

func (ts *OpenshiftTestsSuites) LoadAll() error {
	err := ts.OpenshiftConformance.Load()
	if err != nil {
		return err
	}
	err = ts.KubernetesConformance.Load()
	if err != nil {
		return err
	}
	return nil
}

func (ts *OpenshiftTestsSuites) GetTotalOCP() int {
	return ts.OpenshiftConformance.Count
}

func (ts *OpenshiftTestsSuites) GetTotalK8S() int {
	return ts.KubernetesConformance.Count
}

type OpenshiftTestsSuite struct {
	InputFile string
	Name      string
	Count     int
	Tests     []string
}

func (s *OpenshiftTestsSuite) Load() error {
	content, err := os.ReadFile(s.InputFile)
	if err != nil {
		log.Fatal(err)
		return err
	}

	s.Tests = strings.Split(string(content), "\n")
	s.Count = len(s.Tests)
	return nil
}
