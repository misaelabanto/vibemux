package gitstatus_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/misaelabanto/vibemux/internal/gitstatus"
)

// gitAvailable returns true if git is on PATH.
func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// initRepo sets up a bare git repo with one committed file and returns its path.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")

	// Create and commit a file so HEAD exists.
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", "README")
	run("commit", "-m", "init")

	return dir
}

func TestNonRepo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	s := gitstatus.Compute(dir)
	if s.IsRepo {
		t.Errorf("expected IsRepo=false for non-repo dir, got true")
	}
}

func TestCleanRepo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := initRepo(t)
	s := gitstatus.Compute(dir)
	if !s.IsRepo {
		t.Errorf("expected IsRepo=true")
	}
	if !s.Clean {
		t.Errorf("expected Clean=true, got %+v", s)
	}
	if s.Modified || s.Staged || s.Untracked || s.Conflict || s.Stashed {
		t.Errorf("expected all dirty flags false, got %+v", s)
	}
}

func TestUntracked(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := initRepo(t)

	// Create an untracked file.
	if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("untracked\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := gitstatus.Compute(dir)
	if !s.IsRepo {
		t.Errorf("expected IsRepo=true")
	}
	if !s.Untracked {
		t.Errorf("expected Untracked=true, got %+v", s)
	}
	if s.Clean {
		t.Errorf("expected Clean=false when Untracked, got %+v", s)
	}
}

func TestStaged(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := initRepo(t)

	// Create and stage a new file.
	f := filepath.Join(dir, "staged.txt")
	if err := os.WriteFile(f, []byte("staged\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	s := gitstatus.Compute(dir)
	if !s.IsRepo {
		t.Errorf("expected IsRepo=true")
	}
	if !s.Staged {
		t.Errorf("expected Staged=true, got %+v", s)
	}
	if s.Clean {
		t.Errorf("expected Clean=false when Staged, got %+v", s)
	}
}

func TestModified(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := initRepo(t)

	// Modify the already-committed file without staging.
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("modified\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := gitstatus.Compute(dir)
	if !s.IsRepo {
		t.Errorf("expected IsRepo=true")
	}
	if !s.Modified {
		t.Errorf("expected Modified=true, got %+v", s)
	}
	if s.Clean {
		t.Errorf("expected Clean=false when Modified, got %+v", s)
	}
}

func TestStashed(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := initRepo(t)

	// Modify a file and stash it.
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("stashed change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "stash")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git stash: %v\n%s", err, out)
	}

	s := gitstatus.Compute(dir)
	if !s.IsRepo {
		t.Errorf("expected IsRepo=true")
	}
	if !s.Stashed {
		t.Errorf("expected Stashed=true, got %+v", s)
	}
	if s.Clean {
		t.Errorf("expected Clean=false when Stashed, got %+v", s)
	}
}

func TestConflict(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := initRepo(t)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runAllowFail := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Ignore error - merge conflict exits non-zero intentionally.
		_ = cmd.Run()
	}

	// Create a divergent change on a feature branch vs main so that merging
	// produces a conflict on the same line.
	f := filepath.Join(dir, "README")

	// main: overwrite README with "main content"
	if err := os.WriteFile(f, []byte("main content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "main change")

	// Create branch-a from the initial commit (one commit back).
	run("checkout", "-b", "branch-a", "HEAD~1")
	if err := os.WriteFile(f, []byte("branch content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "branch change")

	// Merge main into branch-a; this will conflict.
	runAllowFail("merge", "main")

	s := gitstatus.Compute(dir)
	if !s.IsRepo {
		t.Errorf("expected IsRepo=true")
	}
	if !s.Conflict {
		t.Errorf("expected Conflict=true, got %+v", s)
	}
	if s.Clean {
		t.Errorf("expected Clean=false when Conflict, got %+v", s)
	}
}

func TestAheadBehind(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}

	// Create a bare repo to act as "origin".
	bareDir := t.TempDir()
	bareCmd := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = bareDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v (bare): %v\n%s", args, err, out)
		}
	}
	bareCmd("init", "--bare")

	// Clone the bare repo into a work directory.
	workDir := t.TempDir()
	cloneCmd := exec.Command("git", "clone", bareDir, workDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	workRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v (work): %v\n%s", args, err, out)
		}
	}

	// Configure identity in work repo.
	workRun("config", "user.email", "test@example.com")
	workRun("config", "user.name", "Test User")

	// Create an initial commit and push so origin/main exists.
	f := filepath.Join(workDir, "README")
	if err := os.WriteFile(f, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	workRun("add", "README")
	workRun("commit", "-m", "init")
	workRun("push", "-u", "origin", "HEAD")

	// Make one more local commit without pushing.
	if err := os.WriteFile(f, []byte("local change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	workRun("add", "README")
	workRun("commit", "-m", "local ahead")

	s := gitstatus.Compute(workDir)
	if !s.IsRepo {
		t.Errorf("expected IsRepo=true")
	}
	if !s.HasUpstream {
		t.Errorf("expected HasUpstream=true, got %+v", s)
	}
	if s.Ahead < 1 {
		t.Errorf("expected Ahead>=1, got %+v", s)
	}
	if s.Behind != 0 {
		t.Errorf("expected Behind=0, got %+v", s)
	}
}
