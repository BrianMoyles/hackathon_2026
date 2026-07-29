package providerdiff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"compatibility-lab/internal/model"
	"compatibility-lab/internal/scanner/provider"
)

// CompareRefs materializes two git refs of a provider repo into
// temporary worktrees, runs provider.Scan against each, and returns the
// two manifests plus a cleanup function. The caller MUST invoke the
// cleanup even on error to remove the worktrees; passing a nil cleanup
// back would strand disk space and leave stale entries in
// `git worktree list`.
//
// Why worktrees and not a fresh clone? A worktree is O(worktree size)
// on disk instead of O(repo history), and it shares the object store
// with the source repo — which matters for a repo the size of
// terraform-provider-genesyscloud. We use --detach so the temporary
// worktree does not create a named branch the user has to clean up
// themselves if this process is killed mid-run.
func CompareRefs(providerRepo, baseRef, headRef string) (baseManifest, headManifest model.ProviderManifest, cleanup func(), err error) {
	cleanup = func() {}
	if _, statErr := os.Stat(filepath.Join(providerRepo, ".git")); statErr != nil {
		return baseManifest, headManifest, cleanup, fmt.Errorf("%s does not look like a git repository: %w", providerRepo, statErr)
	}

	tmpBase, err := os.MkdirTemp("", "compatlab-base-*")
	if err != nil {
		return baseManifest, headManifest, cleanup, fmt.Errorf("create base worktree dir: %w", err)
	}
	tmpHead, err := os.MkdirTemp("", "compatlab-head-*")
	if err != nil {
		_ = os.RemoveAll(tmpBase)
		return baseManifest, headManifest, cleanup, fmt.Errorf("create head worktree dir: %w", err)
	}

	// Register a best-effort cleanup that always runs, even if scan
	// fails halfway through. `git worktree remove --force` is the
	// safest cleanup because it also unregisters the worktree in
	// .git/worktrees/; a naive os.RemoveAll would leave orphan
	// pointers behind.
	cleanup = func() {
		_ = runGit(providerRepo, "worktree", "remove", "--force", tmpBase)
		_ = runGit(providerRepo, "worktree", "remove", "--force", tmpHead)
		_ = os.RemoveAll(tmpBase)
		_ = os.RemoveAll(tmpHead)
	}

	if err := runGit(providerRepo, "worktree", "add", "--detach", tmpBase, baseRef); err != nil {
		return baseManifest, headManifest, cleanup, fmt.Errorf("git worktree add base %q: %w", baseRef, err)
	}
	if err := runGit(providerRepo, "worktree", "add", "--detach", tmpHead, headRef); err != nil {
		return baseManifest, headManifest, cleanup, fmt.Errorf("git worktree add head %q: %w", headRef, err)
	}

	baseManifest, err = provider.Scan(tmpBase)
	if err != nil {
		return baseManifest, headManifest, cleanup, fmt.Errorf("scan base %q: %w", baseRef, err)
	}
	headManifest, err = provider.Scan(tmpHead)
	if err != nil {
		return baseManifest, headManifest, cleanup, fmt.Errorf("scan head %q: %w", headRef, err)
	}

	// Overwrite RepoPath so the JSON output records the git ref rather
	// than a throwaway temp directory that will not exist after
	// cleanup.
	baseManifest.RepoPath = fmt.Sprintf("%s@%s", providerRepo, baseRef)
	headManifest.RepoPath = fmt.Sprintf("%s@%s", providerRepo, headRef)

	return baseManifest, headManifest, cleanup, nil
}

// runGit is a thin exec wrapper that surfaces stderr in the returned
// error. We deliberately avoid streaming stdout back to the caller — the
// only git commands we run (worktree add / remove) do not print output
// callers care about, and swallowing stdout keeps `--format json` clean.
func runGit(repo string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w (%s)", args, err, string(output))
	}
	return nil
}
