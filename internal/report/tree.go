package main

import (
	"cmp"
	"encoding/json"
	"io"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/onsi/ginkgo/v2/types"
	"k8s.io/klog/v2"
)

// SuiteTree represents a tree of test suites. Suites are indentified by their path in in file system.
type SuiteTree struct {
	// Path is the absolute path to the test suite.
	Path string
	// Name is the directory name of the test suite. It is the last element of the path.
	Name string
	// Description is the description of the test suite. It is taken from the report.
	Description string
	// Specs is the sum of specs from all child suites, recursively.
	Specs int
	// Children is a list of child suites. It can be sorted by [SuiteTree.Sort].
	Children []*SuiteTree
	// SpecReport should only be set when Children is empty, meaning this is a leaf node representing a single spec.
	// It should only have It specs.
	SpecReport *types.SpecReport
}

// NewFromReports creates a new SuiteTree from a list of reports. The root of the tree will be `/`.
func NewFromReports(reports []types.Report) *SuiteTree {
	klog.V(100).Infof("Creating SuiteTree from %d reports", len(reports))

	root := &SuiteTree{
		Path: "/",
		Name: "",
	}

	for _, report := range reports {
		leaf := root.Insert(report.SuitePath, report.SuiteDescription, report.PreRunStats.TotalSpecs)
		leaf.InsertSpecs(report.SpecReports)
	}

	return root
}

// NewFromFile creates a new SuiteTree from a Ginkgo report file. The root of the tree will be `/`.
func NewFromFile(path string) (*SuiteTree, error) {
	klog.V(100).Infof("Creating SuiteTree from Ginkgo JSON report at path %s", path)

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	reportsBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	reports := []types.Report{}

	err = json.Unmarshal(reportsBytes, &reports)
	if err != nil {
		return nil, err
	}

	return NewFromReports(reports), nil
}

// Insert adds a new suite to the tree as a leaf node and returns the node that was added. It will return nil if the
// suitePath does not start with the tree's path.
//
// This assumes that all insertions are leaf nodes, so if a tree contains an internal node with the same path as the
// inserted suite, the existing children will be lost.
func (tree *SuiteTree) Insert(suitePath, description string, specs int) *SuiteTree {
	klog.V(100).Infof("Inserting suite %s with %d specs", suitePath, specs)

	if !strings.HasPrefix(suitePath, tree.Path) {
		klog.V(100).Infof("Skipping suite %s because it does not start with the tree's path %s", suitePath, tree.Path)

		return nil
	}

	splitPath := strings.Split(suitePath, "/")
	currNode := tree

	for _, elem := range splitPath {
		currNode.Specs += specs

		child := currNode.findChild(elem)
		if child == nil {
			klog.V(100).Infof("Creating child %s in tree with path %s", elem, currNode.Path)

			child = &SuiteTree{
				Path: path.Join(currNode.Path, elem),
				Name: elem,
			}
			currNode.Children = append(currNode.Children, child)
		}

		currNode = child
	}

	currNode.Specs = specs
	currNode.Description = description
	currNode.Children = nil

	return currNode
}

// InsertSpecs will add the spec reports to the receiver's children. This assumes that the receiver is a leaf node.
func (tree *SuiteTree) InsertSpecs(specs types.SpecReports) {
	klog.V(100).Infof("Inserting specs into suite %s", tree.Path)

	specs = specs.WithLeafNodeType(types.NodeTypeIt)
	for _, spec := range specs {
		text := "It " + spec.LeafNodeText
		copiedSpec := spec
		child := &SuiteTree{
			Path:       path.Join(tree.Path, text),
			Name:       text,
			Specs:      1,
			SpecReport: &copiedSpec,
		}
		tree.Children = append(tree.Children, child)
	}
}

// Sort sorts the children of the tree first by the number of specs and then by name. If descending is true, the
// children are sorted in descending order by number of specs, but the name is still sorted alphabetically.
func (tree *SuiteTree) Sort(descending bool) {
	klog.V(100).Infof("Sorting tree with path %s", tree.Path)

	if len(tree.Children) == 0 {
		return
	}

	slices.SortFunc(tree.Children, func(treeA, treeB *SuiteTree) int {
		factor := 1
		if descending {
			factor = -1
		}

		if n := factor * cmp.Compare(treeA.Specs, treeB.Specs); n != 0 {
			return n
		}

		return strings.Compare(treeA.Name, treeB.Name)
	})

	for _, child := range tree.Children {
		child.Sort(descending)
	}
}

// TrimRoot recurively removes the root node until it encounters a node with more than one child. Note that this will
// likely prevent the tree from being inserted into anymore.
func (tree *SuiteTree) TrimRoot() *SuiteTree {
	klog.V(100).Infof("Trimming root of tree with path %s", tree.Path)

	for len(tree.Children) == 1 {
		tree = tree.Children[0]
	}

	klog.V(100).Infof("Trimmed root of tree to path %s", tree.Path)

	return tree
}

// String returns a string representation of the tree. It contains one line per node and is indented with a dot and two
// spaces per level.
func (tree *SuiteTree) String() string {
	builder := &strings.Builder{}
	tree.stringLevel(builder, 0)

	return builder.String()
}

// stringLevel is a helper function to recursively build the string representation of the tree. The level parameter is
// used to control the indentation and starts at 0.
func (tree *SuiteTree) stringLevel(builder *strings.Builder, level uint) {
	for range level {
		builder.WriteString(".  ")
	}

	builder.WriteString(tree.Name)
	builder.WriteByte(' ')
	builder.WriteString(strconv.Itoa(tree.Specs))
	builder.WriteByte('\n')

	for _, child := range tree.Children {
		child.stringLevel(builder, level+1)
	}
}

// findChild returns the child with the given name or nil if no child with that name exists. It only searches direct
// children of the tree.
func (tree *SuiteTree) findChild(name string) *SuiteTree {
	klog.V(100).Infof("Searching for child %s in tree with path %s", name, tree.Path)

	for _, child := range tree.Children {
		if child.Name == name {
			return child
		}
	}

	klog.V(100).Infof("No child with name %s found in tree with path %s", name, tree.Path)

	return nil
}
