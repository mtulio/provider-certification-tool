package retrieve

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	sonobuoyclient "github.com/vmware-tanzu/sonobuoy/pkg/client"
	config2 "github.com/vmware-tanzu/sonobuoy/pkg/config"
	"golang.org/x/sync/errgroup"

	"github.com/redhat-openshift-ecosystem/provider-certification-tool/pkg"
	"github.com/redhat-openshift-ecosystem/provider-certification-tool/pkg/client"
	"github.com/redhat-openshift-ecosystem/provider-certification-tool/pkg/status"
)

type RetrieveOptions struct {
	Silent bool
}

var options RetrieveOptions

func NewCmdRetrieve() *cobra.Command {
	// opts := &RetrieveOptions{}

	cmd := &cobra.Command{
		Use:   "retrieve",
		Args:  cobra.MaximumNArgs(1),
		Short: "Collect results from certification environment",
		Long:  `Downloads the results archive from the certification environment`,
		Run: func(cmd *cobra.Command, args []string) {
			destinationDirectory, err := os.Getwd()
			if err != nil {
				log.Error(err)
				return
			}
			if len(args) == 1 {
				destinationDirectory = args[0]
				finfo, err := os.Stat(destinationDirectory)
				if err != nil {
					log.Error(err)
					return
				}
				if !finfo.IsDir() {
					log.Error("Retrieval destination must be directory")
					return
				}
			}

			kclient, sclient, err := client.CreateClients()
			if err != nil {
				log.Error(err)
				return
			}

			s := status.NewStatusOptions(false)
			err = s.PreRunCheck(kclient)
			if err != nil {
				log.Error(err)
				return
			}

			if !options.Silent {
				log.Info("Collecting results...")
			}

			if err := retrieveResultsRetry(sclient, destinationDirectory); err != nil {
				log.Error(err)
				return
			}

			if !options.Silent {
				log.Info("Use the results command to check the certification test summary or share the results archive with your Red Hat partner.")
			}
		},
	}

	cmd.Flags().BoolVarP(&options.Silent, "silent", "s", false, "Run in silent mode, printing only the path to results.")

	return cmd
}

func retrieveResultsRetry(sclient sonobuoyclient.Interface, destinationDirectory string) error {
	var err error
	limit := 10 // Retry retrieve 10 times
	pause := time.Second * 2
	retries := 1
	for retries <= limit {
		err = retrieveResults(sclient, destinationDirectory)
		if err != nil {
			log.Warn(err)
			if retries+1 < limit {
				if !options.Silent {
					log.Warnf("Retrying retrieval %d more times", limit-retries)
				}
			}
			time.Sleep(pause)
			retries++
			continue
		}
		return nil // Retrieved results without a problem
	}

	return errors.New("Retrieval retry limit reached")
}

func retrieveResults(sclient sonobuoyclient.Interface, destinationDirectory string) error {
	// Get a reader that contains the tar output of the results directory.
	reader, ec, err := sclient.RetrieveResults(&sonobuoyclient.RetrieveConfig{
		Namespace: pkg.CertificationNamespace,
		Path:      config2.AggregatorResultsPath,
	})
	if err != nil {
		return errors.Wrap(err, "error retrieving results from sonobuoy")
	}

	// Download results into target directory
	results, err := writeResultsToDirectory(destinationDirectory, reader, ec)
	if err != nil {
		return errors.Wrap(err, "error retrieving certification results from sonobyuoy")
	}

	// Log the new files to stdout
	for _, result := range results {
		if options.Silent {
			fmt.Print(result)
		} else {
			log.Infof("Results saved to %s", result)
		}
	}

	return nil
}

func writeResultsToDirectory(outputDir string, r io.Reader, ec <-chan error) ([]string, error) {
	eg := &errgroup.Group{}
	var results []string
	eg.Go(func() error { return <-ec })
	eg.Go(func() error {
		// This untars the request itself, which is tar'd as just part of the API request, not the sonobuoy logic.
		filesCreated, err := sonobuoyclient.UntarAll(r, outputDir, "")
		if err != nil {
			return err
		}
		// Only print the filename if not extracting. Allows capturing the filename for scripting.
		results = append(results, filesCreated...)

		return nil
	})

	return results, eg.Wait()
}
