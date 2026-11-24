---
version: 1
slug: "sc-web-src-app-tsx"
primary_target: "sc/web/src/App.tsx"
related_targets: ["sc/web/src/screens"]
---

# cockatoo app (every screen)

Mode: Operate. Audience: people with busy mailboxes, several accounts, open all day on Windows (light and dark follow Windows). Task: see what came in, deal with it on the row, let the AI point at what needs them. Constraints: flat (every action on its row), the AI suggests and the user decides, plain words, not cartoony, not busy, not Gmail or Outlook. Logo and tagline are in.

## Direction contract

THESIS: Your mail as a printed page you mark up: only what matters gets the yellow highlighter swipe, and what you have dealt with gets a pen tick. Refuses the category default of sidebar, list and reading pane in grey with a blue accent.

OWN-WORLD: Paper-white sheet on a warm desk ground; navy ink (the logo wordmark) for text; one chisel-tip highlighter yellow (the crest) reserved for what needs you; blue ballpoint for the AI's margin notes and ticks; done green for handled mail. Public Sans for everything; a handwriting face only for pen marks (AI notes, ticks). Dark: the same sheet under a desk lamp, deep ink ground, highlighter as a glow-free yellow swipe.

STORY: The user opens cockatoo, sees one sheet: what needs them is highlighted at the top with a pen note saying why, the rest below in calm ink, done mail ticked and quiet. They act on the row and tick it off.

FIRST VIEWPORT: Header bar: bird mark and lowercase cockatoo left, flat nav right. Centred sheet, about 920px: Inbox heading, the span and counts, search underlined in ink, account tabs, then "Needs you now" rows with highlighted subjects and pen notes, then everything else. Row actions stay on the right of every row.

FORM: Highlighted Printout, position 1 on the ordered list (taken as IMPECCABLE'S PICK over the roll's assignment). seed:0f2815113019ff09

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Confirmed decisions

- 2026-11-24, the user: "Keep both". Mail the AI picks shows in "What needs you now" and again, highlighted, in the last-24-hours list, so the list stays complete.
- The conversation keeps a left/right chat layout because the user asked for it: "a chat interface will be great".
