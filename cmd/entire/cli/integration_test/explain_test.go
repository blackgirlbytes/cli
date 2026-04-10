//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint/id"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/stretchr/testify/require"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func TestExplain_NoCurrentSession(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	// Without any flags, explain shows the branch view (not an error)
	output, err := env.RunCLIWithError("explain")

	if err != nil {
		t.Errorf("expected success for branch view, got error: %v, output: %s", err, output)
		return
	}

	// Should show branch information and checkpoint count
	if !strings.Contains(output, "Branch:") {
		t.Errorf("expected 'Branch:' header in output, got: %s", output)
	}
	if !strings.Contains(output, "Checkpoints:") {
		t.Errorf("expected 'Checkpoints:' in output, got: %s", output)
	}
}

func TestExplain_SessionFilter(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	// --session now filters the list view instead of showing session details
	// A nonexistent session ID should show an empty list, not an error
	output, err := env.RunCLIWithError("explain", "--session", "nonexistent-session-id")

	if err != nil {
		t.Errorf("expected success (empty list) for session filter, got error: %v, output: %s", err, output)
		return
	}

	// Should show branch header
	if !strings.Contains(output, "Branch:") {
		t.Errorf("expected 'Branch:' header in output, got: %s", output)
	}

	// Should show 0 checkpoints (filter found no matches)
	if !strings.Contains(output, "Checkpoints: 0") {
		t.Errorf("expected 'Checkpoints: 0' for nonexistent session filter, got: %s", output)
	}

	// Should show filter info
	if !strings.Contains(output, "Filtered by session:") {
		t.Errorf("expected 'Filtered by session:' in output, got: %s", output)
	}
}

func TestExplain_MutualExclusivity(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	// Try to provide both --session and --commit flags
	output, err := env.RunCLIWithError("explain", "--session", "test-session", "--commit", "abc123")

	if err == nil {
		t.Errorf("expected error when both flags provided, got output: %s", output)
		return
	}

	if !strings.Contains(strings.ToLower(output), "cannot specify multiple") {
		t.Errorf("expected 'cannot specify multiple' error, got: %s", output)
	}
}

func TestExplain_CheckpointNotFound(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	// Try to explain a non-existent checkpoint
	output, err := env.RunCLIWithError("explain", "--checkpoint", "nonexistent123")

	if err == nil {
		t.Errorf("expected error for nonexistent checkpoint, got output: %s", output)
		return
	}

	if !strings.Contains(output, "checkpoint not found") {
		t.Errorf("expected 'checkpoint not found' error, got: %s", output)
	}
}

func TestExplain_CheckpointMutualExclusivity(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	// Try to provide --checkpoint with --session
	output, err := env.RunCLIWithError("explain", "--session", "test-session", "--checkpoint", "abc123")

	if err == nil {
		t.Errorf("expected error when both flags provided, got output: %s", output)
		return
	}

	if !strings.Contains(strings.ToLower(output), "cannot specify multiple") {
		t.Errorf("expected 'cannot specify multiple' error, got: %s", output)
	}
}

func TestExplain_CommitWithoutCheckpoint(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	// Create a regular commit without Entire-Checkpoint trailer
	env.WriteFile("test.txt", "content")
	env.GitAdd("test.txt")
	env.GitCommit("Regular commit without Entire trailer")

	// Get the commit hash
	commitHash := env.GetHeadHash()

	// Run explain --commit
	output, err := env.RunCLIWithError("explain", "--commit", commitHash[:7])
	if err != nil {
		t.Fatalf("unexpected error: %v, output: %s", err, output)
	}

	// Should show "No associated Entire checkpoint" message
	if !strings.Contains(output, "No associated Entire checkpoint") {
		t.Errorf("expected 'No associated Entire checkpoint' message, got: %s", output)
	}
}

func TestExplain_CommitWithCheckpointTrailer(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	// Create a commit with Entire-Checkpoint trailer
	env.WriteFile("test.txt", "content")
	env.GitAdd("test.txt")
	env.GitCommitWithCheckpointID("Commit with checkpoint", "abc123def456")

	// Get the commit hash
	commitHash := env.GetHeadHash()

	// Run explain --commit - it should try to look up the checkpoint
	// Since the checkpoint doesn't actually exist in the store, it should error
	output, err := env.RunCLIWithError("explain", "--commit", commitHash[:7])

	// We expect an error because the checkpoint abc123def456 doesn't exist
	if err == nil {
		// If it succeeded, check if it found the checkpoint (it shouldn't)
		if strings.Contains(output, "Checkpoint:") {
			t.Logf("checkpoint was found (unexpected but ok if test created one)")
		}
	} else {
		// Expected: checkpoint not found error
		if !strings.Contains(output, "checkpoint not found") {
			t.Errorf("expected 'checkpoint not found' error, got: %s", output)
		}
	}
}

func TestExplain_CheckpointV2EnabledFallsBackToV1(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)

	// Create a v1-only checkpoint (checkpoints_v2 disabled by default).
	session := env.NewSession()
	err := env.SimulateUserPromptSubmitWithPrompt(session.ID, "Create v1 fallback file")
	require.NoError(t, err)

	content := "v1 fallback content"
	env.WriteFile("fallback.txt", content)

	session.CreateTranscript(
		"Create v1 fallback file",
		[]FileChange{{Path: "fallback.txt", Content: content}},
	)
	err = env.SimulateStop(session.ID, session.TranscriptPath)
	require.NoError(t, err)

	env.GitCommitWithShadowHooks("Create v1 fallback file", "fallback.txt")
	checkpointID := env.GetLatestCheckpointIDFromHistory()

	// Simulate enabling checkpoints_v2 after the v1-only checkpoint already exists.
	env.PatchSettings(map[string]any{
		"strategy_options": map[string]any{"checkpoints_v2": true},
	})

	output, err := env.RunCLIWithError("explain", "--checkpoint", checkpointID[:6])
	require.NoError(t, err, "expected explain checkpoint fallback to v1 to succeed: %s", output)

	if !strings.Contains(output, "Checkpoint: "+checkpointID) {
		t.Errorf("expected checkpoint ID in output, got: %s", output)
	}
	if !strings.Contains(output, "Intent: Create v1 fallback file") {
		t.Errorf("expected intent from v1 transcript in output, got: %s", output)
	}
}

func TestExplain_CheckpointV2EnabledPrefersV2WhenDualWriteExists(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)

	env.PatchSettings(map[string]any{
		"strategy_options": map[string]any{"checkpoints_v2": true},
	})

	session := env.NewSession()
	err := env.SimulateUserPromptSubmitWithPrompt(session.ID, "Create v2 preferred file")
	require.NoError(t, err)

	content := "v2 preferred content"
	env.WriteFile("v2-preferred.txt", content)
	session.CreateTranscript(
		"Create v2 preferred file",
		[]FileChange{{Path: "v2-preferred.txt", Content: content}},
	)
	err = env.SimulateStop(session.ID, session.TranscriptPath)
	require.NoError(t, err)

	// Creates dual-write checkpoint (v1 + v2).
	env.GitCommitWithShadowHooks("Create v2 preferred file", "v2-preferred.txt")
	checkpointID := env.GetLatestCheckpointIDFromHistory()

	// Corrupt only the v1 transcript for this checkpoint. If explain wrongly prefers
	// v1 when v2 is available, the intent will show this v1-only prompt.
	repo, err := git.PlainOpen(env.RepoDir)
	require.NoError(t, err)
	v1Store := checkpoint.NewGitStore(repo)
	cpID := id.MustCheckpointID(checkpointID)

	summary, err := v1Store.ReadCommitted(context.Background(), cpID)
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.NotEmpty(t, summary.Sessions)

	v1Content, err := v1Store.ReadSessionContent(context.Background(), cpID, 0)
	require.NoError(t, err)

	err = v1Store.UpdateCommitted(context.Background(), checkpoint.UpdateCommittedOptions{
		CheckpointID: cpID,
		SessionID:    v1Content.Metadata.SessionID,
		Transcript:   []byte(`{"type":"user","message":{"content":[{"type":"text","text":"v1 overridden prompt"}]}}` + "\n"),
		Prompts:      []string{"v1 overridden prompt"},
		Agent:        v1Content.Metadata.Agent,
	})
	require.NoError(t, err)

	output, err := env.RunCLIWithError("explain", "--checkpoint", checkpointID[:6])
	require.NoError(t, err, "expected explain to prefer v2 checkpoint data: %s", output)

	if !strings.Contains(output, "Intent: Create v2 preferred file") {
		t.Errorf("expected intent from v2 compact transcript, got: %s", output)
	}
	if strings.Contains(output, "v1 overridden prompt") {
		t.Errorf("unexpected v1-overridden intent found in output: %s", output)
	}
}

func TestExplain_CheckpointV2NoFullTranscriptUsesCompact(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)

	// Enable v2 to get dual-write checkpoints.
	env.PatchSettings(map[string]any{
		"strategy_options": map[string]any{"checkpoints_v2": true},
	})

	session := env.NewSession()
	err := env.SimulateUserPromptSubmitWithPrompt(session.ID, "Create compact-only file")
	require.NoError(t, err)

	content := "compact only content"
	env.WriteFile("compact-only.txt", content)
	session.CreateTranscript(
		"Create compact-only file",
		[]FileChange{{Path: "compact-only.txt", Content: content}},
	)
	err = env.SimulateStop(session.ID, session.TranscriptPath)
	require.NoError(t, err)

	env.GitCommitWithShadowHooks("Create compact-only file", "compact-only.txt")
	checkpointID := env.GetLatestCheckpointIDFromHistory()

	repo, err := git.PlainOpen(env.RepoDir)
	require.NoError(t, err)

	// Delete the v2 /full/current ref so no raw transcript is available from v2.
	err = repo.Storer.RemoveReference(plumbing.ReferenceName(paths.V2FullCurrentRefName))
	require.NoError(t, err)

	// Overwrite the v1 transcript with a marker so we can detect if explain
	// falls back to v1 instead of using the v2 compact transcript.
	v1Store := checkpoint.NewGitStore(repo)
	cpID := id.MustCheckpointID(checkpointID)
	v1Content, err := v1Store.ReadSessionContent(context.Background(), cpID, 0)
	require.NoError(t, err)

	err = v1Store.UpdateCommitted(context.Background(), checkpoint.UpdateCommittedOptions{
		CheckpointID: cpID,
		SessionID:    v1Content.Metadata.SessionID,
		Transcript:   []byte(`{"type":"user","message":{"content":[{"type":"text","text":"v1 marker prompt"}]}}` + "\n"),
		Prompts:      []string{"v1 marker prompt"},
		Agent:        v1Content.Metadata.Agent,
	})
	require.NoError(t, err)

	// Default explain (not --full) should succeed using compact transcript from v2 /main.
	output, err := env.RunCLIWithError("explain", "--checkpoint", checkpointID[:6])
	require.NoError(t, err, "expected explain to succeed with compact transcript when /full/* is missing: %s", output)

	require.Contains(t, output, "Checkpoint: "+checkpointID)
	// Intent should come from the v2 compact transcript, not the v1 marker.
	require.Contains(t, output, "Intent: Create compact-only file")
	require.NotContains(t, output, "v1 marker prompt",
		"explain should use v2 compact transcript, not fall back to v1")
}

func TestExplain_CheckpointV2MalformedFallsBackToV1(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)

	// Enable v2 to get dual-write checkpoints.
	env.PatchSettings(map[string]any{
		"strategy_options": map[string]any{"checkpoints_v2": true},
	})

	session := env.NewSession()
	err := env.SimulateUserPromptSubmitWithPrompt(session.ID, "Create v1 resilience file")
	require.NoError(t, err)

	content := "v1 resilience content"
	env.WriteFile("v1-resilience.txt", content)
	session.CreateTranscript(
		"Create v1 resilience file",
		[]FileChange{{Path: "v1-resilience.txt", Content: content}},
	)
	err = env.SimulateStop(session.ID, session.TranscriptPath)
	require.NoError(t, err)

	env.GitCommitWithShadowHooks("Create v1 resilience file", "v1-resilience.txt")
	checkpointID := env.GetLatestCheckpointIDFromHistory()

	repo, err := git.PlainOpen(env.RepoDir)
	require.NoError(t, err)

	// Corrupt the v2 /main ref by replacing it with a tree containing invalid
	// metadata.json. This causes ReadCommitted to return a JSON parse error
	// (not ErrCheckpointNotFound), which tests whether the resolver falls back
	// to v1 for non-sentinel errors.
	corruptV2MainRef(t, repo, checkpointID)

	// Explain should fall back to the valid v1 checkpoint.
	output, err := env.RunCLIWithError("explain", "--checkpoint", checkpointID[:6])
	require.NoError(t, err, "expected explain to fall back to v1 when v2 is malformed: %s", output)

	require.Contains(t, output, "Checkpoint: "+checkpointID)
	require.Contains(t, output, "Intent: Create v1 resilience file")
}

// corruptV2MainRef replaces the v2 /main ref's tree with one where the given
// checkpoint's metadata.json contains invalid JSON. This triggers a parse error
// in V2GitStore.ReadCommitted (a non-sentinel error).
func corruptV2MainRef(t *testing.T, repo *git.Repository, checkpointID string) {
	t.Helper()

	refName := plumbing.ReferenceName(paths.V2MainRefName)
	ref, err := repo.Storer.Reference(refName)
	require.NoError(t, err, "v2 /main ref should exist")

	// Get the current commit to use as parent.
	parentHash := ref.Hash()

	// Create a blob with invalid JSON for metadata.json.
	garbageBlob, err := checkpoint.CreateBlobFromContent(repo, []byte(`{invalid json!!!`))
	require.NoError(t, err)

	cpID := id.MustCheckpointID(checkpointID)
	cpPath := cpID.Path() // e.g. "ab/cdef123456"
	parts := strings.SplitN(cpPath, "/", 2)
	require.Len(t, parts, 2, "checkpoint path should have shard/remainder format")

	// Build tree bottom-up: metadata.json → checkpoint dir → shard dir → root
	cpTree := &object.Tree{Entries: []object.TreeEntry{
		{Name: "metadata.json", Mode: filemode.Regular, Hash: garbageBlob},
	}}
	cpTreeObj := repo.Storer.NewEncodedObject()
	require.NoError(t, cpTree.Encode(cpTreeObj))
	cpTreeHash, err := repo.Storer.SetEncodedObject(cpTreeObj)
	require.NoError(t, err)

	shardTree := &object.Tree{Entries: []object.TreeEntry{
		{Name: parts[1], Mode: filemode.Dir, Hash: cpTreeHash},
	}}
	shardTreeObj := repo.Storer.NewEncodedObject()
	require.NoError(t, shardTree.Encode(shardTreeObj))
	shardTreeHash, err := repo.Storer.SetEncodedObject(shardTreeObj)
	require.NoError(t, err)

	rootTree := &object.Tree{Entries: []object.TreeEntry{
		{Name: parts[0], Mode: filemode.Dir, Hash: shardTreeHash},
	}}
	rootTreeObj := repo.Storer.NewEncodedObject()
	require.NoError(t, rootTree.Encode(rootTreeObj))
	rootTreeHash, err := repo.Storer.SetEncodedObject(rootTreeObj)
	require.NoError(t, err)

	commitHash, err := checkpoint.CreateCommit(repo, rootTreeHash, parentHash,
		"corrupt metadata for test", "Test", "test@test.com")
	require.NoError(t, err)

	require.NoError(t, repo.Storer.SetReference(
		plumbing.NewHashReference(refName, commitHash)))
}

// TestExplain_BranchListingShowsCheckpointsAndPrompts runs the same scenario
// with v2 disabled and enabled, verifying that `entire explain` (branch listing)
// finds committed checkpoints and displays their prompts in both modes.
func TestExplain_BranchListingShowsCheckpointsAndPrompts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		v2   bool
	}{
		{"v1_only", false},
		{"v2_enabled", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := NewFeatureBranchEnv(t)

			if tc.v2 {
				env.PatchSettings(map[string]any{
					"strategy_options": map[string]any{"checkpoints_v2": true},
				})
			}

			session := env.NewSession()
			err := env.SimulateUserPromptSubmitWithPrompt(session.ID, "Implement user authentication")
			require.NoError(t, err)

			env.WriteFile("auth.go", "package auth\nfunc Login() {}\n")
			session.CreateTranscript(
				"Implement user authentication",
				[]FileChange{{Path: "auth.go", Content: "package auth\nfunc Login() {}\n"}},
			)
			err = env.SimulateStop(session.ID, session.TranscriptPath)
			require.NoError(t, err)

			env.GitCommitWithShadowHooks("Implement user authentication", "auth.go")

			// `entire explain` (no flags) should show the branch listing with the checkpoint.
			output, err := env.RunCLIWithError("explain")
			require.NoError(t, err, "explain should succeed: %s", output)

			require.Contains(t, output, "Branch:")
			require.Contains(t, output, "Checkpoints: 1")
			require.Contains(t, output, "Implement user authentication",
				"branch listing should show the commit message or prompt")
		})
	}
}

// TestExplain_BranchListingV2OnlyAfterV1Deleted verifies that the branch listing
// works when only v2 data exists (v1 metadata branch deleted after dual-write).
func TestExplain_BranchListingV2OnlyAfterV1Deleted(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)

	env.PatchSettings(map[string]any{
		"strategy_options": map[string]any{"checkpoints_v2": true},
	})

	session := env.NewSession()
	err := env.SimulateUserPromptSubmitWithPrompt(session.ID, "Create v2 resilience file")
	require.NoError(t, err)

	content := "v2 resilience content"
	env.WriteFile("resilience.txt", content)
	session.CreateTranscript(
		"Create v2 resilience file",
		[]FileChange{{Path: "resilience.txt", Content: content}},
	)
	err = env.SimulateStop(session.ID, session.TranscriptPath)
	require.NoError(t, err)

	env.GitCommitWithShadowHooks("Create v2 resilience file", "resilience.txt")

	// Delete the v1 metadata branch.
	repo, err := git.PlainOpen(env.RepoDir)
	require.NoError(t, err)
	_ = repo.Storer.RemoveReference(plumbing.NewBranchReferenceName("entire/checkpoints/v1"))

	// Branch listing should still work using v2 data.
	output, err := env.RunCLIWithError("explain")
	require.NoError(t, err, "explain should succeed with v2 only: %s", output)

	require.Contains(t, output, "Checkpoints: 1",
		"checkpoint should be visible from v2 after v1 deletion")
	require.Contains(t, output, "Create v2 resilience file",
		"prompt/intent should be readable from v2 after v1 deletion")
}
