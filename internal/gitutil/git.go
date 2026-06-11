package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// StageInfo describes one unmerged index stage for a path.
type StageInfo struct {
	Mode  string
	Stage int
	Path  string
}

// RepoRoot returns the repository root directory for the given working directory.
func RepoRoot(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", gitCommandError("git rev-parse --show-toplevel", err, stderr.String())
	}
	root := trimCommandLineEnding(string(output))
	if root == "" {
		return "", fmt.Errorf("git rev-parse returned empty repo root")
	}
	return root, nil
}

// ListUnmergedFiles returns repo-relative paths of conflicted files under scopePathspec.
func ListUnmergedFiles(ctx context.Context, repoRoot string, scopePathspec string) ([]string, error) {
	pathspec := literalPathspec(scopePathspec)

	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", "--diff-filter=U", "--", pathspec)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, gitCommandError("git diff --name-only -z --diff-filter=U", err, stderr.String())
	}

	return splitNULPaths(output), nil
}

// UnmergedStages returns all unmerged index stages for a single repo-relative path.
func UnmergedStages(ctx context.Context, repoRoot string, path string) (map[int]StageInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-u", "-z", "--", literalPathspec(path))
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, gitCommandError("git ls-files -u -z", err, stderr.String())
	}

	stages := map[int]StageInfo{}
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}

		meta, rawPath, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("git ls-files -u returned malformed record %q", string(record))
		}

		fields := strings.Fields(string(meta))
		if len(fields) != 3 {
			return nil, fmt.Errorf("git ls-files -u returned malformed metadata %q", string(meta))
		}

		stage, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("git ls-files -u returned invalid stage %q: %w", fields[2], err)
		}

		stages[stage] = StageInfo{Mode: fields[0], Stage: stage, Path: string(rawPath)}
	}
	return stages, nil
}

// ShowStage reads a conflicted file content from the git index stage (1=base, 2=ours, 3=theirs).
func ShowStage(ctx context.Context, repoRoot string, stage int, path string) ([]byte, error) {
	ref := fmt.Sprintf(":%d:%s", stage, path)
	cmd := exec.CommandContext(ctx, "git", "show", ref)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, gitCommandError(fmt.Sprintf("git show %s", ref), err, stderr.String())
	}
	return output, nil
}

func splitNULPaths(output []byte) []string {
	if len(output) == 0 {
		return nil
	}

	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		paths = append(paths, string(part))
	}
	return paths
}

func literalPathspec(pathspec string) string {
	if pathspec == "" || pathspec == "." {
		return "."
	}
	return ":(literal)" + pathspec
}

func trimCommandLineEnding(output string) string {
	output = strings.TrimSuffix(output, "\n")
	return strings.TrimSuffix(output, "\r")
}

func gitCommandError(command string, err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("%s failed: %w", command, err)
	}
	return fmt.Errorf("%s failed: %s: %w", command, stderr, err)
}
