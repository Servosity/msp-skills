# td-synnex skill - governance and safety model

> Unofficial. Community-built skill for the TD SYNNEX StreamOne ION API. Not
> affiliated with, endorsed by, or sponsored by TD SYNNEX Corporation.
> This page tells an MSP owner exactly what the td-synnex skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `td-synnex-cli` binary (and `td-synnex-mcp`) as the StreamOne ION
user whose OAuth credentials you request in the ION portal (**Settings > Users > Edit >
OAuth credentials > Request New Credentials**). It can do whatever that user can do in
your reseller account, and nothing more.

- **Refresh token.** You pipe it once to `td-synnex-cli auth mint --refresh-token-stdin`
  (or set `TD_SYNNEX_REFRESH_TOKEN` before the first mint). ION refresh tokens are
  single-use and last 32 days; every exchange returns a new one.
- **Where tokens are kept.** `auth mint` saves the access token (2 hours) and the new
  refresh token to a private credentials file under the CLI's data directory (`0600` on
  macOS and Linux; on Windows the CLI refuses to load a credentials file whose owner or
  ACL lets another user read it), under a file lock, before it prints anything. If that save fails it writes the new refresh token to a private
  rescue file next to the config so you are not locked out. `TD_SYNNEX_ACCESS_TOKEN`
  in the environment overrides the saved access token.
- **Never printed.** Token values are printed only when you pass `auth mint
  --print-access-token`. `auth set-token` stores an access token and keeps the stored
  refresh token; `auth logout` clears both.
- **Where tokens go.** Only to the ION API: `https://ion.tdsynnex.com` unless you set
  `TD_SYNNEX_BASE_URL` for a run (for tests), which is never saved to the config file.
  Every request, API call or token exchange, refuses plain HTTP unless it goes to a
  loopback test server, and refuses a `base_url` from the config file (or `--config`)
  that points anywhere other than ion.tdsynnex.com or a loopback test server unless
  `TD_SYNNEX_BASE_URL` is set for the run.
- **Account.** `TD_SYNNEX_ACCOUNT_ID` fills `{accountId}` in every API path.

## Default-safe behavior

- **`subscriptions set` previews by default.** It reads the subscription live, prints
  the exact UPDATE order it would place (through the same redactor as ION responses),
  and places nothing until you add `--confirm`.
- **Every other write sends immediately.** `--dry-run` is opt-in: it prints the request
  and sends nothing; the request body is printed through the same redactor as ION
  responses, so an order carrying a tenant admin password shows it as `[redacted]`. Make your agent's policy: preview with `--dry-run`, show the exact
  command, get approval, then run the write.
- **The licence-book commands are local reads.** `subscriptions renewals`,
  `subscriptions changes`, `margin`, `position export`, `position trueup`, `orders stuck`
  and `customers show` read the synced SQLite store and cannot change ION.
- **Credentials in ION data are blanked.** Every ION response passes through one redactor before the response cache, the synced store, printed output, `--deliver` sinks or MCP tool results see it: secret-named fields, secret-named name/value order attributes, credential objects (only identity fields survive: user name, login, email, domain, UPN and login URL), credentials inside JSON strings such as `azureCustomerInfo`, and credential text, withheld whole, judged after decoding HTML, percent and `\u` escapes to any depth and folding look-alike spaces, dashes, colons and arrows to plain ones, typographic apostrophes and accent look-alikes to `'` and the slash look-alikes ∕ ⁄ ⧸ ╱ (and fullwidth ／) to `/`, and dropping every invisible (default-ignorable) character such as zero-width joiners, variation selectors and Hangul fillers (a letter swapped for a look-alike from another script, such as Cyrillic а, is not recognized). In free text (notes, descriptions, activity details, report CSV) a mention of any credential word (password, passcode, PIN, OTP, secret, token, credential, API key, recovery code, creds, p/w, pswd, passkey, door or access code (PW and PSK alone are not labels: they occur in part numbers), and these translations: Passwort/Passwörter, Kennwort/Kennwörter, Zugangsdaten, Anmeldedaten, Zugangscode, API-Schlüssel, Geheimnis; mot de passe, mdp, identifiants, jeton, code d'accès, clé API, clé secrète; contraseña, clave, credenciales, código de acceso, llave API; senha, credenciais, chave API, segredo; wachtwoord, geheim; credenziali, chiave API, segreto; no other language is recognized) or secret field name withholds the whole string, so a note such as "Your password is never stored", or a CSV report with a "Duo Hardware Token" row, comes back as `[redacted]` (`reports get-data` keeps such a report cell by cell). Name, ID and address fields (customer names and organizations, product names, SKUs, part numbers, street addresses) use a narrower label rule so those names survive. In every field a tag, URL or SIP login, token query parameter or secret-named table column (punctuation and any script included) also counts. A JSON property name that is not a plain field name (it holds a space, a colon or other punctuation) is judged as text too: if it names a secret or mentions a credential, the pair is withheld whole under a `[redacted]` key. A password typed into a note with no credential word near it, or into a plain field name with no separator, cannot be recognized. The only exemption is the OAuth token exchange that `auth mint` and `auth check` make. The cost: `customers create` and `cloud-providers provision-customer` cannot show the one-time password ION generates.
- **No MCP tool takes a file path.** Flags that read or write a file (`teach
  --playbook-file`, `teach-playbook --notes-file`, `position trueup --with` / `--billed`,
  `--db`) are dropped from every MCP tool and refused as arguments, so an agent cannot
  read your credentials file back through them. The file true-up runs from the CLI.
- **MCP endpoint write tools take only the arguments they declare.** They have no
  dry-run: an endpoint tool that sends a POST, PUT or DELETE (every write, and the
  report and pricing reads ION serves over POST) refuses any argument it does not
  declare (`dry-run` included) and sends nothing. Preview a write with the CLI's
  `--dry-run`; the tools built from CLI commands, such as `subscriptions_set`, do take a
  `dry-run` argument.
- **MCP read-only mode.** `TD_SYNNEX_MCP_READ_ONLY=1` (any value except blank, `0`,
  `false`, `no` or `off`) removes every MCP tool that can change ION: order create and cancel, cart checkout and edits, customer create and
  update, product enable and disable, provisioning and linking, report save and delete,
  and `subscriptions set`, plus the tools that record learnings (`teach`,
  `teach_playbook`, `teach_lookup`, `teach_pattern`, `learnings_confirm`, `learnings_forget`,
  `learnings_reject`, `playbook_amend`, `workflow_archive`) and `sync`. The tools it keeps
  still write local state (the synced store when a read refreshes it, the command journal
  and recall statistics).
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating - the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**; require a
human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Lists, gets, report runs and the local licence book. No change to ION. | `subscriptions renewals`, `subscriptions changes`, `margin`, `position export`, `position trueup`, `orders stuck`, `customers show`, every `list` / `get`, `orders`, `pricing`, `cloud-profiles`, `cloud-providers get-customer`, `reports get` / `get-data` / `get-data-csv` / `get-details` / `get-metadata`, `sync`, `search`, `doctor`, `auth check`, `auth status` | Allow |
| **Write (routine)** | Creates and edits records that do not order anything. | `customers create`, `customers update`, `carts create`, `carts update`, `cart-items create`, `cart-items update`, `reports save` | Preview with `--dry-run`, then an approved write |
| **Orders and spend** | Places billable orders or changes what a customer is subscribed to. | `subscriptions set` (seats, renewal setting, friendly name, tier), `carts checkout`, `customer-orders create-order`, `cloud-providers provision-customer`, `cloud-providers link-customer`, `products enable`, `products disable`, `programs batch-update-product` | Human-in-the-loop only |
| **Destructive** | Cancels, suspends, deletes. | `subscriptions set --status suspended` or `deleted`, `subscriptions set --renewal cancel_at_renewal` (or its alias `auto-off`), `customer-orders cancel-order`, `carts delete`, `cart-items delete`, `reports delete` | Human-in-the-loop only, explicit confirmation |
| **Credential** | Mints, stores or clears ION tokens. | `auth mint`, `auth set-token`, `auth logout` | Human-in-the-loop only |

Microsoft's own rules still apply to every order: NCE seat decreases and cancellations
are only accepted inside Microsoft's window after a term starts or renews.

## How to lock it down

- **Issue the OAuth credentials to the right ION user.** A read and reporting workflow
  does not need a user that can place orders. The ION user's role is the real boundary.
- **Keep autonomous agents to Read + previewed writes**, and run the MCP server with
  `TD_SYNNEX_MCP_READ_ONLY=1` when the agent only needs answers.
- **Never let an agent run Orders, Destructive or Credential tier commands unattended.**
  Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credentials if they are ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md): request new OAuth
  credentials in the ION portal and run `auth mint` with them.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the StreamOne ION API, and you can read every line of how it does so.
