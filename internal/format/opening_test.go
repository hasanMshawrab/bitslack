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
		ID:     1,
		Title:  "Add feature X",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feature/add-x"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
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
	assertContains(t, rendered, "*Title:* Add feature X")
	assertContains(t, rendered, "*[my-repo] Pull Request")
	assertContains(t, rendered, "<https://bitbucket.org/myworkspace/my-repo/pull-requests/1|#1>")
	// New format: source → destination in header
	assertContains(t, rendered, "feature/add-x → main")
	// Old labeled field dropped
	assertNotContains(t, rendered, "*Repository:*")
}

func TestOpeningMessage_WithUnmappedUsers(t *testing.T) {
	pr := &event.PullRequest{
		ID:     2,
		Title:  "Fix bug Y",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "fix/bug-y"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
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
		ID:     3,
		Title:  "Refactor Z",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "refactor/z"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
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
		ID:     4,
		Title:  "Solo PR",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "solo"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
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
		ID:     5,
		Title:  "Partial PR",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "partial"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
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

func TestOpeningMessage_ApprovedReviewerCheckmark(t *testing.T) {
	pr := &event.PullRequest{
		ID:     6,
		Title:  "Approved PR",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feat/x"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: []event.User{
			{Nickname: "bobreviewer", AccountID: "acct-bob"},
			{Nickname: "carolreviewer", AccountID: "acct-carol"},
		},
		Participants: []event.Participant{
			{AccountID: "acct-bob", Nickname: "bobreviewer", Role: "REVIEWER", Approved: true},
			{AccountID: "acct-carol", Nickname: "carolreviewer", Role: "REVIEWER", Approved: false},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/6",
	}
	resolve := mapResolver(map[string]string{
		"acct-jane":  "U001JANE",
		"acct-bob":   "U002BOB",
		"acct-carol": "U003CAROL",
	})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	// Bob approved — should show ✅ before his mention
	assertContains(t, rendered, "✅ <@U002BOB>")
	// Carol pending — no ✅ before her mention
	assertContains(t, rendered, "<@U003CAROL>")
	assertNotContains(t, rendered, "✅ <@U003CAROL>")
}

func TestOpeningMessage_AlsoApproved(t *testing.T) {
	pr := &event.PullRequest{
		ID:     7,
		Title:  "PR with observer approval",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feat/y"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: []event.User{
			{Nickname: "bobreviewer", AccountID: "acct-bob"},
		},
		Participants: []event.Participant{
			{AccountID: "acct-bob", Nickname: "bobreviewer", Role: "REVIEWER", Approved: false},
			{AccountID: "acct-dave", Nickname: "daveobserver", Role: "PARTICIPANT", Approved: true},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/7",
	}
	resolve := mapResolver(map[string]string{
		"acct-jane": "U001JANE",
		"acct-bob":  "U002BOB",
		"acct-dave": "U004DAVE",
	})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertContains(t, rendered, "*Also approved:*")
	assertContains(t, rendered, "<@U004DAVE>")
}

func TestOpeningMessage_NoAlsoApproved_WhenNoParticipantApproval(t *testing.T) {
	pr := &event.PullRequest{
		ID:     8,
		Title:  "PR no observers",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feat/z"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Reviewers: []event.User{
			{Nickname: "bobreviewer", AccountID: "acct-bob"},
		},
		Participants: []event.Participant{
			{AccountID: "acct-bob", Nickname: "bobreviewer", Role: "REVIEWER", Approved: true},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/8",
	}
	resolve := mapResolver(map[string]string{
		"acct-jane": "U001JANE",
		"acct-bob":  "U002BOB",
	})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertNotContains(t, rendered, "*Also approved:*")
}

func TestOpeningMessage_ClickUpTicket(t *testing.T) {
	pr := &event.PullRequest{
		ID:          9,
		Title:       "Ticket PR",
		Description: "Implements https://app.clickup.com/t/abc123def and more details.",
		Author:      event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feat/ticket"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
			Repository: event.Repository{Name: "my-repo"},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/9",
	}
	resolve := mapResolver(map[string]string{"acct-jane": "U001JANE"})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertContains(t, rendered, "*Ticket:* <https://app.clickup.com/t/abc123def|View Ticket>")
}

func TestOpeningMessage_ClickUpTicket_MarkdownLink(t *testing.T) {
	// PR description uses Bitbucket markdown link syntax wrapping the ClickUp URL.
	// The regex must not include the ](url) markdown scaffolding in the extracted URL.
	pr := &event.PullRequest{
		ID:          9,
		Title:       "Ticket PR markdown",
		Description: "[https://app.clickup.com/t/zz9876xy](https://app.clickup.com/t/zz9876xy){: data-inline-card='' }",
		Author:      event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feat/ticket"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
			Repository: event.Repository{Name: "my-repo"},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/9",
	}
	resolve := mapResolver(map[string]string{"acct-jane": "U001JANE"})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertContains(t, rendered, "*Ticket:* <https://app.clickup.com/t/zz9876xy|View Ticket>")
}

func TestOpeningMessage_NoTicket_WhenNoClickUpURL(t *testing.T) {
	pr := &event.PullRequest{
		ID:          10,
		Title:       "No ticket PR",
		Description: "Just some description without a ClickUp link.",
		Author:      event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feat/no-ticket"},
			Repository: event.Repository{Name: "my-repo"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
			Repository: event.Repository{Name: "my-repo"},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/my-repo/pull-requests/10",
	}
	resolve := mapResolver(map[string]string{"acct-jane": "U001JANE"})

	_, blocks := format.OpeningMessage(pr, resolve)
	rendered := blocksToText(blocks)

	assertNotContains(t, rendered, "*Ticket:*")
	assertNotContains(t, rendered, "View Ticket")
}

func TestOpeningMessage_HeaderFormat(t *testing.T) {
	pr := &event.PullRequest{
		ID:     42,
		Title:  "Some PR",
		Author: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: "feature/auth"},
			Repository: event.Repository{Name: "payments-api"},
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: "main"},
			Repository: event.Repository{Name: "payments-api"},
		},
		HTMLURL: "https://bitbucket.org/myworkspace/payments-api/pull-requests/42",
	}
	resolve := mapResolver(map[string]string{})

	_, blocks := format.OpeningMessage(pr, resolve)
	// Header is the first block
	if len(blocks) == 0 || blocks[0].Text == nil {
		t.Fatal("expected at least one block with text")
	}
	header := blocks[0].Text.Text
	assertContains(t, header, "🔀")
	assertContains(t, header, "*[payments-api] Pull Request")
	assertContains(t, header, "<https://bitbucket.org/myworkspace/payments-api/pull-requests/42|#42>")
	assertContains(t, header, "feature/auth → main")
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
