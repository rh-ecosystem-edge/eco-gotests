/*
Report is a tool to generate a report of the test suites in a Ginkgo test suite. It will print a tree of the test suites
and the number of specs in each suite. If an output directory is provided, a static site for visualizing the test suites
will be generated.

Upon successful generation of the report the exit code is 0. If any error occurs it will be logged to stderr and the
exit code will be 1.

Usage:

	report [flags]

The flags are:

	-h, -help
		Print this help message

	-a, -action-url string
		URL to the action generating this report. Only necessary with -o. Uses "/" if left blank

	-b, -branch string
		Space-separated list of globs to match branches. Leave blank to use the local directory

	-c, -clean
		Delete the test suite cache and exit without running

	-o, -output string
		Directory to output static site to. Will not be generated if left blank

	-v int
		Log level verbosity for klog. Use 100 for logging all messages or leave blank for none
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	help      bool
	actionURL string
	branch    string
	clean     bool
	output    string
)

//nolint:gochecknoinits // This is a main package so init is fine.
func init() {
	const (
		helpUsage      = "Print this help message"
		actionURLUsage = "URL to the action generating this report. Only necessary with -o. Uses \"/\" if left blank"
		branchUsage    = "Space-separated list of globs to match branches. Leave blank to use the local directory"
		cleanUsage     = "Delete the test suite cache and exit without running"
		outputUsage    = "Directory to output static site to. Will not be generated if left blank"

		defaultHelp      = false
		defaultActionURL = "/"
		defaultBranch    = ""
		defaultClean     = false
		defaultOutput    = ""

		shorthand = " (shorthand)"
	)

	klog.InitFlags(nil)
	klog.EnableContextualLogging(true)
	logf.SetLogger(logr.Discard())

	_ = flag.Set("logtostderr", "true")

	flag.BoolVar(&help, "help", defaultHelp, helpUsage)
	flag.BoolVar(&help, "h", defaultHelp, helpUsage+shorthand)

	flag.StringVar(&actionURL, "action-url", defaultActionURL, actionURLUsage)
	flag.StringVar(&actionURL, "a", defaultActionURL, actionURLUsage+shorthand)

	flag.StringVar(&branch, "branch", defaultBranch, branchUsage)
	flag.StringVar(&branch, "b", defaultBranch, branchUsage+shorthand)

	flag.BoolVar(&clean, "clean", defaultClean, cleanUsage)
	flag.BoolVar(&clean, "c", defaultClean, cleanUsage+shorthand)

	flag.StringVar(&output, "output", defaultOutput, outputUsage)
	flag.StringVar(&output, "o", defaultOutput, outputUsage+shorthand)
}

func main() {
	flag.Parse()

	if help {
		flag.Usage()

		return
	}

	if clean {
		err := CleanCache()
		if err != nil {
			klog.Errorf("Failed to clean cache: %v", err)

			os.Exit(1)
		}

		return
	}

	treeMap, err := getTrees(branch)
	if err != nil {
		klog.Errorf("Failed to get suite trees when branch=\"%s\": %v", branch, err)

		os.Exit(1)
	}

	printTreeMap(treeMap)

	if output != "" {
		err := templateTreeMap(treeMap, output)
		if err != nil {
			klog.Errorf("Failed to template tree map and save to %s: %v", output, err)

			os.Exit(1)
		}
	}
}

func getTrees(branch string) (map[CacheKey]*SuiteTree, error) {
	ctx, cancel := signal.NotifyContext(context.TODO(), os.Interrupt, os.Kill)
	defer cancel()

	cache, err := NewCacheContext(ctx)
	if err != nil {
		return nil, err
	}

	var treeMap map[CacheKey]*SuiteTree

	if branch != "" {
		patterns := strings.Fields(branch)
		treeMap, err = getFromCacheOrClone(ctx, cache, patterns)
	} else {
		treeMap, err = getLocalTreeMap(cache, ".")
	}

	if err != nil {
		return nil, err
	}

	err = cache.Save()
	if err != nil {
		return nil, err
	}

	for key, tree := range treeMap {
		treeMap[key] = tree.TrimRoot()
		treeMap[key].Sort(true)
	}

	return treeMap, nil
}

func printTreeMap(treeMap map[CacheKey]*SuiteTree) {
	for key, tree := range treeMap {
		fmt.Println("---")
		fmt.Printf("Branch %s (%s)\n", key.Branch, key.Revision[:7])
		fmt.Print(tree)
	}
}

func templateTreeMap(treeMap map[CacheKey]*SuiteTree, output string) error {
	err := os.MkdirAll(output, 0755)
	if err != nil {
		return err
	}

	var branchReports []BranchReportConfig

	for key, tree := range treeMap {
		config := TreeTemplateConfig{
			Tree:       tree,
			Generated:  time.Now(),
			Branch:     key.Branch,
			ActionURL:  template.URL(actionURL),
			RepoURL:    RemoteURL,
			TimeFormat: time.RFC3339,
		}
		outputFileName := fmt.Sprintf("report_%s.html", key.Branch)
		outputFilePath := filepath.Join(output, outputFileName)

		err := TemplateTree(config, outputFilePath)
		if err != nil {
			return err
		}

		branchReport := BranchReportConfig{
			Name:          key.Branch,
			ReportFile:    outputFileName,
			Revision:      key.Revision,
			ShortRevision: key.Revision[:7],
		}
		branchReports = append(branchReports, branchReport)
	}

	config := ReportTemplateConfig{
		BranchReports: branchReports,
		Generated:     time.Now(),
		ActionURL:     template.URL(actionURL),
		RepoURL:       RemoteURL,
		TimeFormat:    time.RFC3339,
	}
	outputFilePath := filepath.Join(output, "report.html")

	err = TemplateReport(config, outputFilePath)
	if err != nil {
		return err
	}

	return nil
}

func getLocalTreeMap(cache *Cache, repoPath string) (map[CacheKey]*SuiteTree, error) {
	tree, err := cache.GetOrCreate(repoPath)
	if err != nil {
		klog.Errorf("Failed to get or create SuiteTree from cache: %v", err)

		return nil, err
	}

	key, err := cache.GetKeyFromPath(repoPath)
	if IsMiss(err) {
		treeMap := map[CacheKey]*SuiteTree{{Branch: "local", Revision: "local"}: tree}

		return treeMap, nil
	}

	if err != nil {
		return nil, err
	}

	treeMap := map[CacheKey]*SuiteTree{key: tree}

	return treeMap, nil
}

func getFromCacheOrClone(ctx context.Context, cache *Cache, patterns []string) (map[CacheKey]*SuiteTree, error) {
	treeMap, err := cache.GetRemotePatterns(patterns)
	if err != nil {
		return nil, err
	}

	tempDir := os.TempDir()
	if _, err = os.Stat(tempDir); err != nil {
		return nil, fmt.Errorf("unable to access temp dir to clone repo: %w", err)
	}

	for key, tree := range treeMap {
		if tree != nil {
			continue
		}

		repoPath, err := CloneRepo(ctx, tempDir, RemoteURL, key.Branch)
		if err != nil {
			return nil, err
		}

		tree, err := cache.GetOrCreate(repoPath)
		if err != nil {
			klog.Errorf("Failed to get or create SuiteTree from cache: %v", err)

			return nil, err
		}

		treeMap[key] = tree
	}

	return treeMap, nil
}
