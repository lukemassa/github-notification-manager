package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v66/github"
	"github.com/hako/durafmt"
	"golang.org/x/oauth2"
)

func main() {
	ctx := context.Background()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	notifications, err := fetchAllUnread(ctx, client)
	if err != nil {
		log.Fatalf("error fetching notifications: %v", err)
	}
	if len(notifications) == 0 {
		fmt.Println("No unread notifications.")
		return
	}

	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].GetUpdatedAt().Time.After(notifications[j].GetUpdatedAt().Time)
	})
	slices.Reverse(notifications)

	if len(notifications) == 0 {
		fmt.Println("🎉 No unread notifications!")
		return
	}

	reader := bufio.NewReader(os.Stdin)
	for i := len(notifications) - 1; i >= 0; i-- { // newest first
		n := notifications[i]

		subject := n.GetSubject()
		repo := n.GetRepository()
		fmt.Println("──────────────────────────────")
		if isRenovate(n) {
			fmt.Printf("⚡ Auto Approving: %s\n", subject.GetTitle())
			markAsRead(ctx, client, n)
			continue
		}

		fmt.Printf("🔔  %s (%s)\n", subject.GetTitle(), n.GetID())
		fmt.Printf("Repo: %s\n", repo.GetFullName())
		fmt.Printf("Type: %s\n", subject.GetType())
		fmt.Printf("URL:  %s\n", uiURL(subject.GetURL()))
		fmt.Printf("Updated: %s ago\n", durafmt.Parse(time.Since(n.GetUpdatedAt().Time)).LimitFirstN(2))

		fmt.Print("Mark as read? [y/N]: ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(strings.ToLower(text))

		if text == "y" || text == "yes" {
			markAsRead(ctx, client, n)
		} else {
			fmt.Println("⏭️  Skipped.")
		}
	}

	fmt.Println("✅ Done processing notifications.")
}

func markAsRead(ctx context.Context, client *github.Client, notification *github.Notification) {
	_, err := client.Activity.MarkThreadRead(ctx, notification.GetID())
	if err != nil {
		log.Printf("⚠️  Failed to mark as read: %v\n", err)
	} else {
		fmt.Println("✅ Marked as read.")
	}
}

func isRenovate(notification *github.Notification) bool {
	subject := notification.GetSubject()
	if strings.HasPrefix(subject.GetTitle(), "chore(deps)") {
		return true
	}
	if strings.HasPrefix(subject.GetTitle(), "fix(deps)") {
		return true
	}
	return false
}

func fetchAllUnread(ctx context.Context, client *github.Client) ([]*github.Notification, error) {
	opts := &github.NotificationListOptions{
		All:           false, // unread only
		Participating: false, // include everything, not just threads you’re directly participating in
		ListOptions: github.ListOptions{
			PerPage: 100, // max page size
			Page:    1,
		},
	}

	var all []*github.Notification
	for {
		ns, resp, err := client.Activity.ListRepositoryNotifications(ctx, "runatlantis", "atlantis", opts)
		if err != nil {
			return nil, err
		}
		all = append(all, ns...)

		if resp.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = resp.NextPage
	}
	return all, nil
}

func uiURL(apiURL string) string {
	const prefix = "https://api.github.com/repos/"
	if !strings.HasPrefix(apiURL, prefix) {
		return apiURL // unexpected but better safe than sorry
	}

	path := strings.TrimPrefix(apiURL, prefix)
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return apiURL
	}

	owner, repo, kind := parts[0], parts[1], parts[2]
	repoPath := owner + "/" + repo

	switch kind {
	case "pulls":
		if len(parts) >= 4 {
			return fmt.Sprintf("https://github.com/%s/pull/%s", repoPath, parts[3])
		}
	case "issues":
		if len(parts) >= 4 {
			return fmt.Sprintf("https://github.com/%s/issues/%s", repoPath, parts[3])
		}
	case "commits":
		if len(parts) >= 4 {
			return fmt.Sprintf("https://github.com/%s/commit/%s", repoPath, parts[3])
		}
	case "releases":
		if len(parts) >= 4 {
			return fmt.Sprintf("https://github.com/%s/releases/%s", repoPath, parts[3])
		}
	default:
		// Covers events like "repository", "discussion", etc.
		return fmt.Sprintf("https://github.com/%s", repoPath)
	}

	// fallback to repo homepage if structure is unfamiliar
	return fmt.Sprintf("https://github.com/%s", repoPath)
}
