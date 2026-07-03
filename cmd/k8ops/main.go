/*
Copyright 2024 ggai.dev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main implements the k8ops CLI tool.
// Usage:
//
//	k8ops diagnose --resource pod/my-pod --namespace default
//	k8ops optimize --scope namespace --namespace production
//	k8ops version
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/ggai/k8ops/internal/provider/anthropic"
	_ "github.com/ggai/k8ops/internal/provider/gemini"
	_ "github.com/ggai/k8ops/internal/provider/openai"

	"github.com/ggai/k8ops/internal/agent"
	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/ggai/k8ops/internal/tools/host"
	"github.com/ggai/k8ops/internal/tools/k8s"
	"github.com/ggai/k8ops/internal/tools/remediation"
	"log/slog"

	"k8s.io/client-go/tools/clientcmd"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "diagnose":
		diagnoseCmd(os.Args[2:])
	case "optimize":
		optimizeCmd(os.Args[2:])
	case "version":
		fmt.Printf("k8ops %s\n", version)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`k8ops - Kubernetes AI Operations CLI

Usage:
  k8ops diagnose [flags]    Diagnose a cluster issue
  k8ops optimize [flags]    Get optimization suggestions
  k8ops version             Show version

Diagnose Flags:
  --provider TYPE           AI provider: openai, anthropic, gemini (default: openai)
  --model NAME              Model name (e.g. gpt-4o, claude-3-5-sonnet-20241022)
  --endpoint URL            Custom API endpoint
  --api-key KEY             API key (or set AIOPS_API_KEY env var)
  --resource KIND/NAME      Resource to diagnose (e.g. pod/my-pod)
  --namespace NS            Namespace (default: default)
  --message MSG             Custom diagnostic message

Optimize Flags:
  --scope TYPE              Scope: cluster, namespace, workload (default: cluster)
  --namespace NS            Namespace for namespace/workload scope
  --provider TYPE           AI provider
  --model NAME              Model name
  --api-key KEY             API key
`)
}

func diagnoseCmd(args []string) {
	fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
	providerType := fs.String("provider", "openai", "AI provider")
	model := fs.String("model", "", "Model name")
	endpoint := fs.String("endpoint", "", "Custom endpoint")
	apiKey := fs.String("api-key", os.Getenv("AIOPS_API_KEY"), "API key")
	resource := fs.String("resource", "", "Resource kind/name (e.g. pod/my-pod)")
	namespace := fs.String("namespace", "default", "Namespace")
	message := fs.String("message", "", "Custom diagnostic message")
	maxSteps := fs.Int("max-steps", 15, "Maximum agent steps")
	_ = fs.Parse(args)

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: --api-key or AIOPS_API_KEY env var required")
		os.Exit(1)
	}

	// Build kube client
	kubeClient, err := k8s.NewKubeClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to cluster: %v\n", err)
		os.Exit(1)
	}

	// Get rest config for remediator
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting kubeconfig: %v\n", err)
		os.Exit(1)
	}

	remediator, _ := remediation.NewRemediator(config)

	// Build tool registry
	registry := buildRegistry(kubeClient, remediator)

	// Build message
	if *message == "" {
		if *resource != "" {
			*message = fmt.Sprintf("Please diagnose the issue with %s in namespace %s", *resource, *namespace)
		} else {
			*message = "Please scan the cluster for any issues and provide a health report."
		}
	}

	// Create provider
	prov, err := provider.New(provider.ProviderConfig{
		Type:     *providerType,
		Model:    *model,
		APIKey:   *apiKey,
		Endpoint: *endpoint,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating provider: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Run agent
	agentInst := agent.New(agent.AgentConfig{
		Provider:     prov,
		Registry:     registry,
		SystemPrompt: agent.DiagnosticSystemPrompt(),
		MaxSteps:     *maxSteps,
		Timeout:      180 * time.Second,
	}, logger)

	fmt.Println("Running diagnostic...")
	result, err := agentInst.Run(context.Background(), *message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print steps
	for i, step := range result.Steps {
		fmt.Printf("\n--- Step %d ---\n", i+1)
		if step.Action != "" {
			fmt.Printf("Tool: %s\nArgs: %s\n", step.Action, step.ActionInput)
		}
		if step.Observation != "" {
			obs := step.Observation
			if len(obs) > 500 {
				obs = obs[:500] + "..."
			}
			fmt.Printf("Result: %s\n", obs)
		}
	}

	// Pretty-print the answer
	fmt.Println("\n=== Result ===")
	var prettyJSON any
	if err := json.Unmarshal([]byte(result.Answer), &prettyJSON); err == nil {
		data, _ := json.MarshalIndent(prettyJSON, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println(result.Answer)
	}

	fmt.Printf("\nTokens used: %d (prompt: %d, completion: %d)\n",
		result.TokenUsage.Total, result.TokenUsage.Prompt, result.TokenUsage.Completion)
}

func optimizeCmd(args []string) {
	fs := flag.NewFlagSet("optimize", flag.ExitOnError)
	providerType := fs.String("provider", "openai", "AI provider")
	model := fs.String("model", "", "Model name")
	endpoint := fs.String("endpoint", "", "Custom endpoint")
	apiKey := fs.String("api-key", os.Getenv("AIOPS_API_KEY"), "API key")
	scope := fs.String("scope", "cluster", "Scope: cluster, namespace, workload")
	namespace := fs.String("namespace", "", "Namespace")
	_ = fs.Parse(args)

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: --api-key or AIOPS_API_KEY env var required")
		os.Exit(1)
	}

	kubeClient, err := k8s.NewKubeClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := tools.NewRegistry()
	registry.Register(&k8s.GetResourceTool{Client: kubeClient})
	registry.Register(&k8s.ListResourcesTool{Client: kubeClient})
	registry.Register(&k8s.DescribeResourceTool{Client: kubeClient})
	registry.Register(&k8s.ListAPIResourcesTool{Client: kubeClient})
	registry.Register(&k8s.GetNodesTool{Client: kubeClient})

	prov, err := provider.New(provider.ProviderConfig{
		Type:     *providerType,
		Model:    *model,
		APIKey:   *apiKey,
		Endpoint: *endpoint,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	message := fmt.Sprintf("Analyze the cluster for optimization opportunities. Scope: %s", *scope)
	if *namespace != "" {
		message += fmt.Sprintf(", namespace: %s", *namespace)
	}

	agentInst := agent.New(agent.AgentConfig{
		Provider:     prov,
		Registry:     registry,
		SystemPrompt: agent.OptimizationSystemPrompt(),
		MaxSteps:     10,
		Timeout:      120 * time.Second,
	}, logger)

	fmt.Println("Running optimization analysis...")
	result, err := agentInst.Run(context.Background(), message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Optimization Report ===")
	var prettyJSON any
	if err := json.Unmarshal([]byte(result.Answer), &prettyJSON); err == nil {
		data, _ := json.MarshalIndent(prettyJSON, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println(result.Answer)
	}
}

func buildRegistry(kubeClient *k8s.KubeClient, remediator *remediation.Remediator) *tools.Registry {
	registry := tools.NewRegistry()
	// K8s tools
	registry.Register(&k8s.GetResourceTool{Client: kubeClient})
	registry.Register(&k8s.ListResourcesTool{Client: kubeClient})
	registry.Register(&k8s.DescribeResourceTool{Client: kubeClient})
	registry.Register(&k8s.GetPodLogsTool{Client: kubeClient})
	registry.Register(&k8s.ListAPIResourcesTool{Client: kubeClient})
	registry.Register(&k8s.GetNodesTool{Client: kubeClient})
	registry.Register(&k8s.GetEventsTool{Client: kubeClient})
	registry.Register(&k8s.GetNamespacesTool{Client: kubeClient})
	registry.Register(&k8s.GetTopTool{Client: kubeClient})
	registry.Register(&k8s.GetHPATool{Client: kubeClient})
	registry.Register(&k8s.GetPDBTool{Client: kubeClient})
	registry.Register(&k8s.GetStorageTool{Client: kubeClient})
	registry.Register(&k8s.GetClusterVersionTool{Client: kubeClient})
	registry.Register(&k8s.GetServicesTool{Client: kubeClient})
	registry.Register(&k8s.GetConfigMapTool{Client: kubeClient})
	registry.Register(&k8s.GetIngressTool{Client: kubeClient})
	registry.Register(&k8s.GetNetworkPolicyTool{Client: kubeClient})
	registry.Register(&k8s.GetPodStatusTool{Client: kubeClient})
	// Host tools
	registry.Register(&host.HostExecTool{})
	registry.Register(&host.HostDiskUsageTool{})
	registry.Register(&host.HostNetworkTool{})
	registry.Register(&host.HostProcessTool{})
	registry.Register(&host.HostServiceTool{})
	registry.Register(&host.HostInfoTool{})
	registry.Register(&host.HostDmesgTool{})
	registry.Register(&host.HostContainerRuntimeTool{})
	registry.Register(&host.HostKubeletTool{})
	registry.Register(&host.HostIPTablesTool{})
	registry.Register(&host.HostMountsTool{})
	registry.Register(&host.HostDiskIOTool{})
	registry.Register(&host.HostMemoryInfoTool{})
	// Remediation tools
	registry.Register(&remediation.PatchResourceTool{R: remediator})
	registry.Register(&remediation.ScaleResourceTool{R: remediator})
	registry.Register(&remediation.RestartPodTool{R: remediator})
	registry.Register(&remediation.CordonNodeTool{R: remediator})
	registry.Register(&remediation.DeleteEvictedPodsTool{R: remediator})
	registry.Register(&remediation.ApplyManifestTool{R: remediator})
	return registry
}
