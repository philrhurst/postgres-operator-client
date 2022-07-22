// Copyright 2021 - 2022 Crunchy Data Solutions, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"

	"github.com/crunchydata/postgres-operator-client/internal/util"
)

// Executor calls commands
type Executor func(
	stdin io.Reader, stdout, stderr io.Writer, command ...string,
) error

// newShowCommand returns the show subcommand of the PGO plugin. The 'show' command
// allows you to display particular details related to the PostgreSQL cluster.
func newShowCommand(kubeconfig *genericclioptions.ConfigFlags) *cobra.Command {

	cmdShow := &cobra.Command{
		Use:   "show",
		Short: "Show PostgresCluster details",
		Long:  "Show allows you to display particular details related to the PostgresCluster",
	}

	cmdShow.AddCommand(
		newShowBackupCommand(kubeconfig),
	)

	// No arguments for 'show', but there are arguments for the subcommands, e.g.
	// 'show backup'
	cmdShow.Args = cobra.NoArgs

	return cmdShow
}

// newShowBackupCommand returns the backup subcommand of the show command. The
// 'backup' command displays the output of the 'pgbackrest info' command.
// - https://pgbackrest.org/command.html ('8 Info Command (info)')
func newShowBackupCommand(kubeconfig *genericclioptions.ConfigFlags) *cobra.Command {

	cmdShowBackup := &cobra.Command{
		Use:     "backup",
		Aliases: []string{"backups"},
		Short:   "Show backup information for a PostgresCluster",
		Long:    "Show backup information for a PostgresCluster from 'pgbackrest info' command.",
	}

	cmdShowBackup.Example = `  kubectl pgo show backup hippo
  kubectl pgo show backup hippo --output=json
  kubectl pgo show backup hippo --repoName=repo1
	`

	// Define the command flags.
	// - https://pgbackrest.org/command.html
	// - output: '8.1.1 Output Option (--output)'
	// - repoName: '8.4.1 Set Repository Option (--repo)'
	var output string
	var repoName string
	cmdShowBackup.Flags().StringVarP(&output, "output", "o", "text",
		"output format. types supported: text,json")
	cmdShowBackup.Flags().StringVar(&repoName, "repoName", "",
		"Set the repository name for the command. example: repo1")

	// Limit the number of args, that is, only one cluster name
	cmdShowBackup.Args = cobra.ExactArgs(1)

	// Define the 'show backup' command
	cmdShowBackup.RunE = func(cmd *cobra.Command, args []string) error {

		// The only thing we need is the value after 'repo' which should be an
		// integer. If anything else is provided, we let the pgbackrest command
		// handle validation.
		repoNum := strings.TrimPrefix(repoName, "repo")

		// configure client
		ctx := context.Background()
		config, err := kubeconfig.ToRESTConfig()
		if err != nil {
			return err
		}
		client, err := dynamic.NewForConfig(config)
		if err != nil {
			return err
		}

		// Get the namespace. This will either be from the Kubernetes configuration
		// or from the --namespace (-n) flag.
		configNamespace, _, err := kubeconfig.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return err
		}

		// Get the primary instance Pod by its labels. For a Postgres cluster
		// named 'hippo', we'll use the following:
		//    postgres-operator.crunchydata.com/cluster=hippo
		//    postgres-operator.crunchydata.com/data=postgres
		//    postgres-operator.crunchydata.com/role=master
		pods, err := client.Resource(schema.GroupVersionResource{
			Version: "v1", Resource: "pods",
		}).Namespace(configNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: util.PrimaryInstanceLabels(args[0]),
		})
		if err != nil {
			return err
		}

		if len(pods.Items) != 1 {
			return fmt.Errorf("Primary instance Pod not found.")
		}

		PodExec, err := util.NewPodExecutor(config)
		if err != nil {
			return err
		}

		// Create an executor and attempt to get the pgBackRest info output.
		exec := func(stdin io.Reader, stdout, stderr io.Writer,
			command ...string) error {
			return PodExec(pods.Items[0].GetNamespace(), pods.Items[0].GetName(),
				util.ContainerDatabase, stdin, stdout, stderr, command...)
		}
		stdout, stderr, err := Executor(exec).pgBackRestInfo(output, repoNum)
		if err != nil {
			return err
		}

		// Print the output received.
		cmd.Printf(stdout)
		if stderr != "" {
			cmd.Printf("\nError returned: %s\n", stderr)
		}

		return nil
	}

	return cmdShowBackup
}

// pgBackRestInfo defines a pgBackRest info command with relevant flags set
func (exec Executor) pgBackRestInfo(output, repoNum string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	var command string

	command = "pgbackrest info --output=" + output
	if repoNum != "" {
		command += " --repo=" + repoNum
	}
	err := exec(nil, &stdout, &stderr, "bash", "-ceu", "--", command)

	return stdout.String(), stderr.String(), err
}