package format_test

import (
	"strings"
	"testing"

	"github.com/hasanMshawrab/bitslack/internal/event"
	"github.com/hasanMshawrab/bitslack/internal/format"
	"github.com/hasanMshawrab/bitslack/internal/slack"
)

func mapResolver(m map[string]string) format.UserResolver {
	return func(accountID string) string {
		return m[accountID]
	}
}

func TestOpeningMessage_WithMappedUsers(t *testing.T) {
	pr := &event.PullRequest{
		Title:  "Add feature X",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Destination: event.Endpoint{
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: []event.User{{Nickname: "bobreviewer", AccountID: "acct-bob"}},
		HTMLURL:   "https://bitbucket.org/myworkspace/my-repo/pull-requests/1",
	}
	resolve := mapResolver(map[string]string{
		"acct-jane": "U001JANE",
		"acct-bob":  "U002BOB",
	})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertContains(t, rendered, "<@U001JANE>")
	assertContains(t, rendered, "<@U002BOB>")
	assertContains(t, rendered, "*Add feature X*")
	assertContains(t, rendered, "my-repo")
}

func TestOpeningMessage_WithUnmappedUsers(t *testing.T) {
	pr := &event.PullRequest{
		Title:  "Fix bug Y",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Destination: event.Endpoint{
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: []event.User{{Nickname: "bobreviewer", AccountID: "acct-bob"}},
		HTMLURL:   "https://bitbucket.org/myworkspace/my-repo/pull-requests/2",
	}
	resolve := mapResolver(map[string]string{}) // empty — no mappings

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertContains(t, rendered, "@janeauthor")
	assertContains(t, rendered, "@bobreviewer")
	assertNotContains(t, rendered, "<@")
}

func TestOpeningMessage_MultipleReviewers(t *testing.T) {
	pr := &event.PullRequest{
		Title:  "Refactor Z",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Destination: event.Endpoint{
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: []event.User{
			{Nickname: "bobreviewer", AccountID: "acct-bob"},
			{Nickname: "alicereviewer", AccountID: "acct-alice"},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/3",
	}
	resolve := mapResolver(map[string]string{
		"acct-jane":  "U001JANE",
		"acct-bob":   "U002BOB",
		"acct-alice": "U003ALICE",
	})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertContains(t, rendered, "<@U002BOB>")
	assertContains(t, rendered, "<@U003ALICE>")
}

func TestOpeningMessage_NoReviewers(t *testing.T) {
	pr := &event.PullRequest{
		Title:  "Solo PR",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Destination: event.Endpoint{
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: nil,
		HTMLURL:   "https://bitbucket.org/myworkspace/my-repo/pull-requests/4",
	}
	resolve := mapResolver(map[string]string{"acct-jane": "U001JANE"})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertNotContains(t, rendered, "Reviewers:")
}

func TestOpeningMessage_PartialMapping(t *testing.T) {
	pr := &event.PullRequest{
		Title:  "Partial PR",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Destination: event.Endpoint{
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: []event.User{
			{Nickname: "bobreviewer", AccountID: "acct-bob"},
			{Nickname: "unknownuser", AccountID: "acct-unknown"},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/5",
	}
	resolve := mapResolver(map[string]string{
		"acct-jane": "U001JANE",
		"acct-bob":  "U002BOB",
	})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertContains(t, rendered, "<@U002BOB>")
	assertContains(t, rendered, "@unknownuser")
}

// helpers

func blocksToText(blocks []slack.Block) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != nil {
			parts = append(parts, b.Text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q, got:\n%s", needle, haystack)
	}
}

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("expected output NOT to contain %q, got:\n%s", needle, haystack)
	}
}
