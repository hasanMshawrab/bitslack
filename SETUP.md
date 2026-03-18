# Setup Guide

Step-by-step instructions for obtaining all credentials and IDs required to run bitslack.

---

## 1. Create a Slack App and Get the Bot Token

1. Go to [https://api.slack.com/apps](https://api.slack.com/apps) and click **Create New App → From scratch**
2. Give it a name (e.g. `BitSlack PR Bot`) and select your workspace
3. In the left sidebar, go to **OAuth & Permissions**
4. Under **Scopes → Bot Token Scopes**, add:
   - `chat:write` — post messages to channels the bot has been invited to
   - `chat:write.public` — post to public channels without being invited _(optional)_
5. Scroll up and click **Install to Workspace**, then **Allow**
6. Copy the **Bot User OAuth Token** — it starts with `xoxb-`

> **Note:** If you are installing the app on a company workspace you do not administrate, you may need approval from a Slack admin before the app can be installed.

---

## 2. Get a Slack User ID

To map a Bitbucket user to a Slack @mention, you need their Slack member ID:

1. Open Slack and click on the user's name or avatar
2. Click **View full profile**
3. Click the **⋯ (More)** button
4. Click **Copy member ID**

The ID looks like `U08XXXXXXXXX`.

> **Tip:** You need one ID per team member you want to appear as a real @mention. Users without a mapping will appear as plain text (e.g. `@alice`) instead of a clickable mention.

---

## 3. Get a Slack Channel ID

1. Open the channel in Slack
2. Click the **channel name** at the top to open channel details
3. Scroll to the bottom of the details panel
4. Copy the **Channel ID** — it looks like `C08XXXXXXXXX`

> **Note:** Make sure the bot has been invited to the channel before it can post there. You can do this by typing `/invite @YourBotName` in the channel.

---

## 4. Generate a Bitbucket API Token

1. Go to [https://id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens)
2. Click **Create API token** and give it a label (e.g. `bitslack`)
3. When prompted to select scopes, enable the following under **Read**:
   - `read:repository:bitbucket` — required for all event families
   - `read:pullrequest:bitbucket` — required for all event families
   - `read:pipeline:bitbucket` — required only if you enable `EventFamilyPipeline`
4. Copy the token immediately — it will not be shown again

> **Note:** API tokens use HTTP Basic auth with your Atlassian account email as the username.

Use the token as follows in your `Config`:

```go
bitslack.Config{
    BitbucketUsername: "user@example.com",  // your Atlassian account email
    BitbucketToken:    "YOUR_API_TOKEN",
}
```

---

## 5. Find Bitbucket Account IDs

The `account_id` is the stable identifier to use for Bitbucket → Slack user mapping.

> **Why `account_id` and not `nickname`?** The `nickname` field is inconsistent — Bitbucket webhook payloads and the REST API can return different values for the same user. The `account_id` is always identical across both sources. See [issue #1](https://github.com/hasanMshawrab/bitslack/issues/1) for details.

### Your own account ID

Open the following URL in a browser while logged in to Atlassian:

```
https://id.atlassian.com/gateway/api/me
```

Or call the API directly:

```bash
curl -u "user@example.com:YOUR_TOKEN" \
  "https://api.bitbucket.org/2.0/user"
```

Look for the `"account_id"` field in the response.

### All workspace members

```bash
curl -u "user@example.com:YOUR_TOKEN" \
  "https://api.bitbucket.org/2.0/workspaces/YOUR_WORKSPACE/members" \
  | python3 -c "
import json, sys
data = json.load(sys.stdin)
for m in data['values']:
    u = m['user']
    print(f\"{u['account_id']}  |  {u['display_name']}\")
"
```

This prints a table of `account_id | display_name` for every workspace member. Use the `account_id` as the key in your `ConfigStore.GetSlackUserID` implementation.

---

## 6. Add the Bitbucket Webhook

1. Open your Bitbucket repository
2. Go to **Repository settings** (gear icon in the left sidebar)
3. Under **Workflow**, click **Webhooks → Add webhook**
4. Fill in the form:
   - **Title**: anything descriptive (e.g. `bitslack`)
   - **URL**: your server's webhook endpoint (e.g. `https://your-server.com/webhook`)
   - **Status**: Active
5. Under **Triggers**, choose **Choose from a full list of triggers** and select:

   **Pull Request:**
   - Created
   - Updated
   - Approved
   - Approval removed
   - Merged
   - Declined
   - Comment created

   **Repository:**
   - Build status created
   - Build status updated

6. Click **Save**

> **Tip:** For local development, use [ngrok](https://ngrok.com) to expose your local server with a public URL:
> ```bash
> ngrok http 8080
> ```
> Then use the generated `https://xxxx.ngrok.io` URL as the webhook URL in Bitbucket.
