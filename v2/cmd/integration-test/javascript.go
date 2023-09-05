package main

import (
	"log"

	"github.com/ory/dockertest/v3"
	"github.com/projectdiscovery/nuclei/v2/pkg/testutils"
	"go.uber.org/multierr"
)

var jsTestcases = []TestCaseInfo{
	{Path: "protocols/javascript/redis-pass-brute.yaml", TestCase: &javascriptRedisPassBrute{}},
	{Path: "protocols/javascript/ssh-server-fingerprint.yaml", TestCase: &javascriptSSHServerFingerprint{}},
}

var (
	redisResource *dockertest.Resource
	sshResource   *dockertest.Resource
	pool          *dockertest.Pool
	defaultRetry  = 3
)

type javascriptRedisPassBrute struct{}

func (j *javascriptRedisPassBrute) Execute(filePath string) error {
	if redisResource == nil {
		// skip test as redis is not running
		return nil
	}
	tempPort := redisResource.GetPort("6379/tcp")
	finalURL := "localhost:" + tempPort
	defer purge(redisResource)
	errs := []error{}
	for i := 0; i < defaultRetry; i++ {
		results, err := testutils.RunNucleiTemplateAndGetResults(filePath, finalURL, debug)
		if err != nil {
			return err
		}
		if err := expectResultsCount(results, 1); err == nil {
			return nil
		} else {
			errs = append(errs, err)
		}
	}
	return multierr.Combine(errs...)
}

type javascriptSSHServerFingerprint struct{}

func (j *javascriptSSHServerFingerprint) Execute(filePath string) error {
	if sshResource == nil {
		// skip test as redis is not running
		return nil
	}
	tempPort := sshResource.GetPort("2222/tcp")
	finalURL := "localhost:" + tempPort
	defer purge(sshResource)
	errs := []error{}
	for i := 0; i < defaultRetry; i++ {
		results, err := testutils.RunNucleiTemplateAndGetResults(filePath, finalURL, debug)
		if err != nil {
			return err
		}
		if err := expectResultsCount(results, 1); err == nil {
			return nil
		} else {
			errs = append(errs, err)
		}
	}
	return multierr.Combine(errs...)
}

// purge any given resource if it is not nil
func purge(resource *dockertest.Resource) {
	if resource != nil && pool != nil {
		containerName := resource.Container.Name
		err := pool.Purge(resource)
		if err != nil {
			log.Printf("Could not purge resource: %s", err)
		}
		_ = pool.RemoveContainerByName(containerName)
	}
}

func init() {
	// uses a sensible default on windows (tcp/http) and linux/osx (socket)
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Printf("something went wrong with dockertest: %s", err)
		return
	}

	// uses pool to try to connect to Docker
	err = pool.Client.Ping()
	if err != nil {
		log.Printf("Could not connect to Docker: %s", err)
	}

	// setup a temporary redis instance
	redisResource, err = pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "redis",
		Tag:        "latest",
		Cmd:        []string{"redis-server", "--requirepass", "iamadmin"},
	})
	if err != nil {
		log.Printf("Could not start resource: %s", err)
		return
	}
	// by default expire after 30 sec
	if err := redisResource.Expire(30); err != nil {
		log.Printf("Could not expire resource: %s", err)
	}

	// setup a temporary ssh server
	sshResource, err = pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "lscr.io/linuxserver/openssh-server",
		Tag:        "latest",
		Env: []string{
			"PUID=1000",
			"PGID=1000",
			"TZ=Etc/UTC",
			"PASSWORD_ACCESS=true",
			"USER_NAME=admin",
			"USER_PASSWORD=admin",
		},
	})
	if err != nil {
		log.Printf("Could not start resource: %s", err)
		return
	}
	// by default expire after 30 sec
	if err := sshResource.Expire(30); err != nil {
		log.Printf("Could not expire resource: %s", err)
	}
}