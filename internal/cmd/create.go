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
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
)

// newCreateCommand returns the create subcommand of the PGO plugin.
// Subcommands of create will be use to create objects, backups, etc.
func newCreateCommand(kubeconfig *genericclioptions.ConfigFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a resource",
		Long:  "Create a resource",
	}

	cmd.AddCommand(newCreateClusterCommand(kubeconfig))

	return cmd
}

// newCreateClusterCommand returns the create cluster subcommand.
// create cluster will take a cluster name as an argument and create a basic
// cluster using a kube client
func newCreateClusterCommand(kubeconfig *genericclioptions.ConfigFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "postgrescluster",
		Short: "Create PostgresCluster with a given name",
		Long:  strings.TrimSpace(`Create basic PostgresCluster with a given name.`),
	}

	cmd.Args = cobra.ExactArgs(1)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		clusterName := args[0]

		namespace, _, err := kubeconfig.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return err
		}

		config, err := kubeconfig.ToRESTConfig()
		if err != nil {
			return err
		}

		client, err := dynamic.NewForConfig(config)
		if err != nil {
			return err
		}

		cluster, err := generateUnstructuredClusterYaml(clusterName)
		if err != nil {
			return err
		}

		u, err := client.
			Resource(schema.GroupVersionResource{
				Group:    "postgres-operator.crunchydata.com",
				Version:  "v1beta1",
				Resource: "postgresclusters",
			}).
			Namespace(namespace).
			Create(ctx, cluster, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		cmd.Printf("postgresclusters/%s created\n", u.GetName())

		return nil
	}

	return cmd
}

// generateUnstructuredClusterYaml takes a name and returns a PostgresCluster
// in the unstructured format.
func generateUnstructuredClusterYaml(name string) (*unstructured.Unstructured, error) {
	var cluster unstructured.Unstructured
	err := yaml.Unmarshal([]byte(fmt.Sprintf(`
apiVersion: postgres-operator.crunchydata.com/v1beta1
kind: PostgresCluster
metadata:
  name: %s
spec:
  postgresVersion: 14
  instances:
  - dataVolumeClaimSpec:
      accessModes:
      - "ReadWriteOnce"
      resources:
        requests:
          storage: 1Gi
  backups:
    pgbackrest:
      repos:
      - name: repo1
        volume:
          volumeClaimSpec:
            accessModes:
            - "ReadWriteOnce"
            resources:
              requests:
                storage: 1Gi
`, name)), &cluster)

	if err != nil {
		return nil, err
	}

	return &cluster, nil
}
