package main

import (
	"strings"
	"testing"
)

// clearEnv blanks every variable resolveDataset reads, so a case states its
// whole input rather than inheriting the developer's shell or the CI runner's.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ANIME_DATASET_URL", "ANIME_DATASET_DIR",
		"VERCEL_GIT_REPO_OWNER", "VERCEL_GIT_REPO_SLUG", "VERCEL_GIT_COMMIT_SHA",
	} {
		t.Setenv(k, "")
	}
}

// TestResolveDatasetPrefersTheWorkingTree pins the precedence that keeps a
// checkout authoritative. A deployment's variables are present in a developer's
// environment often enough (a .env, a vercel pull) that "disk wins" has to be
// asserted rather than assumed.
func TestResolveDatasetPrefersTheWorkingTree(t *testing.T) {
	clearEnv(t)
	t.Setenv("VERCEL_GIT_REPO_OWNER", "michael-freling")
	t.Setenv("VERCEL_GIT_REPO_SLUG", "anime-metadata-db")
	t.Setenv("VERCEL_GIT_COMMIT_SHA", "deadbeef")

	// Run from the repository, where walking up finds the committed dataset.
	got, err := resolveDataset("", "")
	if err != nil {
		t.Fatalf("resolveDataset: %v", err)
	}
	if got.dir == "" {
		t.Fatalf("in a checkout, resolveDataset chose %+v, want the directory", got)
	}
	if got.url != "" {
		t.Errorf("chose a URL as well as a directory: %+v", got)
	}
}

// TestResolveDatasetFallsBackToTheDeployedCommit is the case the outage was.
// With no dataset directory anywhere above — which is exactly the shape of a
// Vercel function, whose Go builder ships the binary and not the repository —
// the records have to come from the commit the deployment was built from.
func TestResolveDatasetFallsBackToTheDeployedCommit(t *testing.T) {
	clearEnv(t)
	// Somewhere with no dataset/ in any parent, so findDataset fails the way
	// it does in the deployed function.
	t.Chdir(t.TempDir())
	t.Setenv("VERCEL_GIT_REPO_OWNER", "michael-freling")
	t.Setenv("VERCEL_GIT_REPO_SLUG", "anime-metadata-db")
	t.Setenv("VERCEL_GIT_COMMIT_SHA", "deadbeef")

	got, err := resolveDataset("", "")
	if err != nil {
		t.Fatalf("resolveDataset: %v", err)
	}
	want := "https://raw.githubusercontent.com/michael-freling/anime-metadata-db/deadbeef/dataset"
	if got.url != want {
		t.Errorf("url = %q, want %q", got.url, want)
	}
	if got.dir != "" {
		t.Errorf("chose a directory with none on disk: %+v", got)
	}
}

func TestResolveDatasetHonoursExplicitOverrides(t *testing.T) {
	for _, tc := range []struct {
		name          string
		flagDir       string
		flagURL       string
		env           map[string]string
		wantDir, want string
	}{
		{name: "-dataset-url beats everything", flagURL: "https://example.test/d", want: "https://example.test/d"},
		{name: "ANIME_DATASET_URL beats the walk", env: map[string]string{"ANIME_DATASET_URL": "https://env.test/d"}, want: "https://env.test/d"},
		{name: "-dataset beats the walk", flagDir: "/somewhere/dataset", wantDir: "/somewhere/dataset"},
		{name: "ANIME_DATASET_DIR beats the walk", env: map[string]string{"ANIME_DATASET_DIR": "/env/dataset"}, wantDir: "/env/dataset"},
		{
			name:    "a URL beats a directory",
			flagDir: "/somewhere/dataset",
			flagURL: "https://example.test/d",
			want:    "https://example.test/d",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, err := resolveDataset(tc.flagDir, tc.flagURL)
			if err != nil {
				t.Fatalf("resolveDataset: %v", err)
			}
			if got.url != tc.want {
				t.Errorf("url = %q, want %q", got.url, tc.want)
			}
			if got.dir != tc.wantDir {
				t.Errorf("dir = %q, want %q", got.dir, tc.wantDir)
			}
		})
	}
}

// TestResolveDatasetSaysSoWhenThereIsNothing asserts the no-source case fails
// loudly. Starting anyway and serving an empty catalogue would read to a
// caller as a dataset that had lost its contents.
func TestResolveDatasetSaysSoWhenThereIsNothing(t *testing.T) {
	clearEnv(t)
	t.Chdir(t.TempDir())

	_, err := resolveDataset("", "")
	if err == nil {
		t.Fatal("resolveDataset succeeded with no dataset and no configuration")
	}
	for _, want := range []string{"-dataset", "-dataset-url"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention %s", err, want)
		}
	}
}
