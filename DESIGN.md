---
name: cockatoo
description: Only the emails that matter. Your mail as a printed page you mark up.
colors:
  desk: "oklch(0.948 0.007 85)"
  paper: "oklch(0.988 0.004 90)"
  paper-tint: "oklch(0.952 0.008 85)"
  ink: "oklch(0.255 0.035 265)"
  ink-muted: "oklch(0.5 0.018 265)"
  rule: "oklch(0.905 0.01 85)"
  field-edge: "oklch(0.84 0.013 85)"
  highlighter: "oklch(0.9 0.165 97 / 0.78)"
  highlighter-soft: "oklch(0.92 0.12 97 / 0.42)"
  pen: "oklch(0.48 0.17 263)"
  done: "oklch(0.53 0.12 152)"
  destructive: "oklch(0.55 0.2 27)"
  desk-dark: "oklch(0.165 0.016 265)"
  paper-dark: "oklch(0.222 0.01 85)"
  paper-raised-dark: "oklch(0.25 0.011 85)"
  paper-tint-dark: "oklch(0.28 0.011 85)"
  ink-dark: "oklch(0.935 0.01 90)"
  ink-on-light-dark: "oklch(0.2 0.02 85)"
  ink-muted-dark: "oklch(0.75 0.012 85)"
  rule-dark: "oklch(1 0 0 / 9%)"
  field-edge-dark: "oklch(1 0 0 / 18%)"
  highlighter-dark: "oklch(0.86 0.16 97 / 0.34)"
  highlighter-ink-dark: "oklch(0.95 0.09 98)"
  highlighter-soft-dark: "oklch(0.86 0.16 97 / 0.12)"
  pen-dark: "oklch(0.76 0.12 263)"
  done-dark: "oklch(0.76 0.13 152)"
  destructive-dark: "oklch(0.7 0.17 25)"
typography:
  display:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "2rem"
    fontWeight: 700
    lineHeight: 1.25
    letterSpacing: "-0.025em"
  headline:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "1.6rem"
    fontWeight: 700
    lineHeight: 1.25
    letterSpacing: "-0.02em"
  title:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 700
    lineHeight: 1.556
    letterSpacing: "-0.01em"
  body:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 400
    lineHeight: 1.5
  body-sm:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.4286
  caption:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.333
  label:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 700
    letterSpacing: "0.08em"
  button:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
  button-sm:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 600
  wordmark:
    fontFamily: "Public Sans Variable, Segoe UI, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 700
    letterSpacing: "-0.02em"
  hand:
    fontFamily: "Caveat, Segoe Print, cursive"
    fontSize: "1.25rem"
    fontWeight: 600
    lineHeight: 1.15
  hand-lead:
    fontFamily: "Caveat, Segoe Print, cursive"
    fontSize: "1.35rem"
    fontWeight: 600
    lineHeight: 1.15
rounded:
  sm: "0.3rem"
  md: "0.4rem"
  lg: "0.5rem"
  check: "4px"
  full: "9999px"
spacing:
  row: "0.875rem"
  group: "1.25rem"
  section: "2rem"
  sheet-x: "3rem"
  sheet-y: "2.5rem"
  sheet-x-narrow: "1rem"
  sheet-y-narrow: "1.75rem"
components:
  sheet:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.lg}"
    padding: "2.5rem 3rem"
    width: "60rem"
  sheet-cleanup:
    backgroundColor: "{colors.paper}"
    rounded: "{rounded.lg}"
    width: "76rem"
  header:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    padding: "0.75rem 1.5rem"
  button-primary:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "0 0.875rem"
    height: "2.25rem"
  button-primary-hover:
    backgroundColor: "oklch(0.255 0.035 265 / 0.88)"
  button-outline:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "0 0.875rem"
    height: "2.25rem"
  button-outline-hover:
    backgroundColor: "{colors.paper-tint}"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "0 0.875rem"
    height: "2.25rem"
  button-ghost-hover:
    backgroundColor: "{colors.paper-tint}"
  button-destructive:
    backgroundColor: "oklch(0.55 0.2 27 / 0.1)"
    textColor: "{colors.destructive}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    height: "2.25rem"
  button-sm:
    typography: "{typography.button-sm}"
    rounded: "{rounded.md}"
    padding: "0 0.75rem"
    height: "2rem"
  button-lg:
    typography: "{typography.button}"
    padding: "0 1rem"
    height: "2.5rem"
  nav-item:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: "0 0.875rem"
    height: "2.25rem"
  nav-item-current:
    backgroundColor: "{colors.paper-tint}"
    textColor: "{colors.ink}"
  account-tab:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    rounded: "0"
    padding: "0 0.125rem"
    height: "2.25rem"
  account-tab-current:
    textColor: "{colors.ink}"
  input:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2.25rem"
  input-ruled:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "0"
    padding: "0 2.25rem 0 1.75rem"
    height: "2.75rem"
  checkbox:
    backgroundColor: "{colors.paper}"
    rounded: "{rounded.check}"
    size: "1rem"
  checkbox-checked:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
  highlight-now:
    backgroundColor: "{colors.highlighter}"
    textColor: "{colors.ink}"
  highlight-today:
    backgroundColor: "{colors.highlighter-soft}"
    textColor: "{colors.ink}"
  pen-note:
    textColor: "{colors.pen}"
    typography: "{typography.hand}"
  handled-line:
    textColor: "{colors.done}"
    typography: "{typography.caption}"
  bubble-theirs:
    backgroundColor: "{colors.paper-tint}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "0.75rem 1rem"
  bubble-mine:
    backgroundColor: "color-mix(in oklch, oklch(0.48 0.17 263) 9%, oklch(0.988 0.004 90))"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "0.75rem 1rem"
  dialog:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.lg}"
    padding: "1.5rem"
    width: "24rem"
---

# Design System: cockatoo

## Overview

**Creative North Star: "The Highlighted Printout"**

Every screen is one sheet of paper lying on a desk, and your mail is printed on it in navy ink. Two hands mark it up. The AI marks it the way you would mark a printout: a chisel-tip yellow highlighter across the few subjects that need you, and a blue ballpoint note in the margin saying why. You mark it too: a pen tick and a pen line through what you have dealt with. Everything else stays calm, printed ink. The sheet is the only object with depth; on it, everything is flat and separated by hairline rules, like a ledger.

The system is quiet by default so that its marks carry meaning. Density is that of a well-set printout: one column, generous row height, every action visible on the right of its row, no menus. Public Sans sets everything printed; Caveat appears only where a pen wrote. Dark is the same sheet under a desk lamp: a deep ink-blue desk, a warm dark sheet, cream ink, the highlighter turned into a translucent yellow swipe with light yellow text under it and no glow. The theme follows Windows (`prefers-color-scheme`, applied as a `.dark` class on the root before first paint and again whenever the setting changes).

It refuses the mail-client default of a grey sidebar, a list and a reading pane with a blue accent. It is not Gmail or Outlook, not cartoony and not busy: motion is limited to the moment a mark is first made.

**Key Characteristics:**
- One centred paper sheet on a desk ground; the sheet is the only lifted surface.
- Navy ink for all printed text; muted ink for everything secondary.
- One highlighter yellow, used on the AI's picks only: full stroke for "Now", a lighter stroke for "Today".
- Blue ballpoint for marks made on the page: the AI's notes, ticks, strike-throughs, the caret and focus.
- Done green only for the line that records what you did with a message.
- Public Sans for everything; Caveat 600 for pen notes only.
- Marks draw themselves once (highlighter 520ms, tick 320ms) and are static for anyone who asks for less motion.

## Colors

A warm paper-and-ink palette with three marking colours, each owned by one kind of mark.

### Primary
- **Navy Ink** (`{colors.ink}`; dark: `{colors.ink-dark}`): the logo wordmark's navy. All printed text, the primary button fill, the checked checkbox, the unread dot, the 2px rule under the selection bar and the ruled search field. In dark the ink becomes cream and the primary button turns cream with dark text (`{colors.ink-on-light-dark}`).

### Secondary
- **Highlighter Yellow** (`{colors.highlighter}`; dark: `{colors.highlighter-dark}`): the crest's yellow, translucent so the ink shows through. Text under it stays ink in light; in dark it turns light yellow (`{colors.highlighter-ink-dark}`) so it reads on the darker swipe.
- **Soft Highlighter** (`{colors.highlighter-soft}`; dark: `{colors.highlighter-soft-dark}`): the lighter stroke for picks that can wait until later today.

### Tertiary
- **Ballpoint Blue** (`{colors.pen}`; dark: `{colors.pen-dark}`): the pen. Caveat notes, the pen tick, the strike-through on dealt-with subjects, the focus outline and focus ring, the text caret, the focused edge of ruled fields, the fill of the determinate progress bar, and (mixed 9% into the paper) the tint on your own messages in a conversation.
- **Done Green** (`{colors.done}`; dark: `{colors.done-dark}`): the record of what you did, in the inbox's handled line ("Saved 2 files to … · Replied · Done").
- **Error Red** (`{colors.destructive}`; dark: `{colors.destructive-dark}`): error text, error boxes (a 40% red border) and the tinted destructive button.

### Neutral
- **Desk** (`{colors.desk}`; dark: `{colors.desk-dark}`): the page background around the sheet. In dark, a deep ink blue.
- **Paper** (`{colors.paper}`; dark: `{colors.paper-dark}`): the sheet, the header bar, dialogs, outline buttons and boxed fields. Popovers in dark sit one step lighter (`{colors.paper-raised-dark}`).
- **Paper Tint** (`{colors.paper-tint}`; dark: `{colors.paper-tint-dark}`): hover and current-state fill on ghost buttons and nav items, chosen rows, and other people's message bubbles.
- **Muted Ink** (`{colors.ink-muted}`; dark: `{colors.ink-muted-dark}`): read mail, times, counts, help text, the column heads, inactive nav items and tabs.
- **Rule** (`{colors.rule}`; dark: `{colors.rule-dark}`): hairline dividers between rows, the header's bottom edge, box borders.
- **Field Edge** (`{colors.field-edge}`; dark: `{colors.field-edge-dark}`): the border of boxed inputs, outline buttons and the reply box.

### Named Rules
**The One Highlighter Rule.** The highlighter swipe goes only on the subject of mail the AI says needs you: the full stroke for "Now", the soft stroke for "Today", in the AI's panel and on the same message's row in the list below it. Read or unread, routine, searched or handled mail never gets the swipe; a handled message is struck through instead, even if the AI picked it. The only other place the yellow appears by design is text selection, which is drawn as a highlighter stroke.

**The Ballpoint Rule.** Blue belongs to marks made on the page: notes, ticks, strike-throughs, the caret and focus. It is never a button fill, a link colour or a decorative accent.

**The Ink Not Colour Rule.** Unread is shown with ink, not colour: a navy dot and a semibold sender and subject. Read mail drops to muted ink.

**The Green Is a Record Rule.** Done green states what was done to a message, in words. It is not a success banner or a button colour.

## Typography

**Display Font:** Public Sans Variable (with Segoe UI, system-ui, sans-serif)
**Body Font:** Public Sans Variable (same stack; `--font-heading` and `--font-sans` are the same face)
**Hand Font:** Caveat 600 (with Segoe Print, cursive)

**Character:** A plain, slightly warm grotesque prints the page; a quick, legible handwriting is the pen that marks it. The contrast between the two is the whole typographic idea.

### Hierarchy
- **Display** (700, 2rem, 1.25, -0.025em): the screen heading on the sheet ("Inbox", "Add your Gmail account").
- **Headline** (700, 1.6rem, 1.25, -0.02em, balanced wrap): an opened conversation's subject.
- **Title** (700, 1.125rem, -0.01em): a section heading within a screen ("What needs you now").
- **Body** (400, 0.9375rem, 1.5): senders, subjects, message text. Unread senders are 600; message text in bubbles runs at relaxed leading.
- **Body small** (400, 0.875rem): status lines, notices, form help, the span-and-counts line under the heading.
- **Caption** (400, 0.75rem): times, account names, routine and handled lines, the AI's fine print. Numbers in times and counts use tabular figures.
- **Label / column head** (700, 0.75rem, 0.08em, uppercase, muted ink): the heading of a part of the page, set like a column head on a printout ("Now", "Today", "Waiting for you", "By year").
- **Hand** (Caveat 600, 1.25rem, 1.15, pen blue): the AI's reason under each pick, and the app's own margin notes. The AI's one-line summary sits at 1.35rem; a note beside a small caption drops to 1rem.

### Named Rules
**The Two Hands Rule.** Caveat is the pen and nothing else. It never sets a heading, a button, a field, a number or a list of data, and it is always pen blue.

**The Column Head Rule.** The small uppercase head is a heading in its own right (an `h2` or `h3`), never a label stacked above another heading.

## Layout

The left rail, then one centred column on the desk beside it. The sheet is 60rem wide for every screen, 76rem for Clean up, whose tables need it. It sits 2rem from the top (1.5rem on narrow screens) with 4rem of desk under it, and is padded 2.5rem by 3rem inside.

Below 768px the rail becomes the bar across the top. Below 640px the sheet goes edge to edge: no rounding, 1rem side padding, 1.75rem top and bottom. The nav scrolls sideways rather than wrapping or collapsing into a menu.

Rhythm on the sheet: 2rem between sections, 1.25rem between a heading block and its tabs, search and groups, and list rows with 0.875rem of vertical padding separated by hairline rules. A row reads left to right: checkbox, unread dot or pen tick, then sender and time on one line, the subject below, then any routine, handled or account line. Actions sit in a fixed 7rem column on the right, so times and buttons line up whether a row has one button or two. Below 640px the time moves under the sender.

The bulk-selection bar sticks to the top of the viewport while the list scrolls, on paper, closed by a 2px ink rule. An opened conversation replaces the list on the same sheet at a 48rem reading width; the list stays mounted, hidden, and its scroll position returns on the way back.

**The Row Is the Toolbar Rule.** Every action on a message is a visible button on its row. There are no menus, hover-only actions or context menus; selection only adds a bulk bar for the same actions.

## Elevation & Depth

Depth is physical and singular: a sheet of paper on a desk. The sheet and the dialog cast the sheet shadow; nothing else is lifted. On the sheet, separation is by hairline rules, a tint fill for state, and the 2px ink rule under the selection bar and ruled fields. Primary buttons carry a 1px contact shadow so they sit on the paper like a stamp.

### Shadow Vocabulary
- **Sheet shadow** (light: `0 1px 2px oklch(0.3 0.03 265 / 0.06), 0 14px 36px -18px oklch(0.3 0.03 265 / 0.28)`; dark: `0 1px 2px oklch(0 0 0 / 0.3), 0 18px 40px -20px oklch(0 0 0 / 0.6)`): the sheet and dialogs.
- **Button contact** (`0 1px 2px oklch(0.25 0.035 265 / 0.18)`): primary buttons only.
- **Dialog overlay** (light: navy at 22%, `oklch(0.25 0.035 265 / 0.22)`; dark: black at 45%): dims the desk behind a dialog.

### Named Rules
**The One Sheet Rule.** One lifted object per view: the sheet, or a dialog above it. Cards, rows and panels on the sheet are flat.

## Shapes

Gently rounded, small radii from a 0.5rem base: buttons and fields at 0.4rem, the sheet, dialogs, boxes and message bubbles at 0.5rem, checkboxes at 4px. Bars (the activity bar, progress bar) and the unread dot are fully round. Ruled fields and account tabs have no radius at all: they are lines on the page. The highlighter swipe is the one irregular shape: two overlapping strokes, slightly angled (100deg and 97deg), with feathered, ragged ends and uneven corners (0.2em 0.5em 0.25em 0.55em), cloned across line breaks so each line gets its own stroke. The pen tick is a hand-drawn 20-unit path stroked at 2.3 with round caps and joins.

## Components

### Buttons
Compact and printed: semibold, small type, quiet until they matter.
- **Shape:** gently rounded (0.4rem), 2.25rem tall; small 2rem, large 2.5rem, extra-small 1.5rem for row actions in Clean up's tables. Square icon buttons for row delete (2rem).
- **Primary:** navy ink fill, paper text, 1px contact shadow (none in dark, where the fill is paper-light). The one action that moves work forward on a row or form: Save, Ask AI, Send, Save account.
- **Outline:** paper fill, field-edge border, ink text (transparent fill in dark). The routine actions: Done, Refresh, Mark done, Test connection.
- **Ghost:** no fill until hovered. Nav items, Not done, Clear, Undo, Cancel, row delete, back to Inbox.
- **Destructive:** a 10% red tint with red text (20% in dark). Only for removing cockatoo's own records (an account, the API key, a save folder, a routine). Moving mail to Trash is an outline or ghost button with a trash icon, not this variant.
- **Hover / Focus / Active:** hover fills ghost and outline with paper tint and dims primary to 88%; focus draws a pen-blue border and a 3px pen ring at 50%; pressed buttons drop 1px. Disabled at 50% opacity. Transitions 150ms. A working button shows a spinner and a present-tense label ("Asking…", "Sending…", "Moving to Trash…").

### Navigation
- **The left rail (from 768px):** a 15.5rem column of paper down the left side, full height and fixed while the page scrolls, with a hairline rule on its right. At the top the bird mark (2rem) and the lowercase wordmark. Then the lists under the column head "Your mail": Reply, Attention, Save, Unsorted, each a 2.5rem ghost item at 0.9375rem with its icon (reply arrow, eye, paperclip, inbox tray) and its count of open mail at the right in tabular figures, ink and bold when above zero, muted at zero. Below a hairline rule, the rest of the app as smaller items: Inbox, Clean up, Folders, Rules, Mail accounts, AI. The tagline and build sit at the foot in muted caption. (Replaced the header bar on 2026-11-25, when the app became organised by lists; SCOPE.md, section 8a.)
- **Narrow windows (below 768px):** the same rail becomes a bar across the top: the mark and wordmark, then one row of all the items, lists first with their counts, scrolling sideways. "Mail accounts" shortens to "Accounts".
- **States:** inactive items in muted ink at weight 500; the current item gets a paper-tint fill, ink text and `aria-current="page"`.
- **Account tabs:** text tabs with no box, a 2px underline in ink on the shown account, muted ink otherwise, the unread count beside each name in muted tabular figures. The lists use the same tabs for "Last 30 days / All mail" and for the accounts.

### Lists (Reply, Attention, Save, Unsorted)
- **Row:** as the Inbox row, with the actions on the right. In a list: Not here (ghost, with an x), Reply (in Reply) or Save (in Save), Done, and the trash. In Unsorted and the new-mail strip: the lists to sort into as small ghost buttons with their icons (labels from 1024px). Not here opens a line under the row, "Not Save. Where does it go?", with an outline button per other list and "Not sure". Below the subject, how it got there in muted caption ("Your rule: mail from …", "You put it here", "You took the AI's suggestion").
- **Suggestion:** on an unsorted message the AI's list and reason are a pen note ("Reply: Ann asks you to approve the budget"); the subject wears the highlighter unless the suggestion is "nothing to do"; the suggested list is the one primary (navy) button and confirms in one click.
- **Just in, not sorted:** at the top of every list but Unsorted, a section titled in 1.125rem bold with its count, three rows between hairline rules, and a ghost link to the rest in Unsorted.
- **Mail like it, from now on:** after a move by hand, a bordered panel above the list: "Moved “subject” to Nothing to do. What about mail like it from now on?", a compact field for subject words, outline buttons "Always from address" and "Anyone at domain", and "Only this one". When a wide rule misplaced it, a second part under a hairline: "Keep that rule only for mail with [invoice] in the subject" and "Narrow the rule".
- **Nothing to do** sits fifth in the rail, as a smaller muted item with an eye-off icon.
- **Teach your agent:** the last part of that panel, under a hairline and the column head "Teach your agent". While the agent thinks, a Working line; then its summary as a pen note, a line saying which skill it changes (or that it is new), the whole skill text in an editable boxed field, the primary button Remember, a ghost Don't, and the caption "Written by the AI from this email. Read it before you approve; change it as you like." Once saved, a pen tick and "Remembered in your skill …".
- **Suggestion line:** after the pen reason, "· your skill “…”" in muted caption sans when a skill decided it.

### Skills & rules
- The screen leads with "Your agent's skills": each skill as its name (semibold), "For …" in muted ink, the instructions at body size in a prose measure, and a caption saying who made this version (a starter, your words, or learned from your correction). Edit opens the same boxed form inline; Add a skill opens it above the list. The rules follow under a hairline, grouped by list.

### Inputs / Fields
- **Boxed field:** paper fill, field-edge border, 0.4rem radius, 2.25rem tall; transparent in dark. Focus: pen border and 3px pen ring. Disabled: 50% opacity on a field-edge tint. Invalid: red border and ring.
- **Ruled field:** for search and the reply text, the field is a line on the page: no box, a 2px rule in ink at 70%, turning pen blue on focus with no ring. Search carries a leading magnifier (a spinner while searching) and a clear button.
- **Caret:** pen blue everywhere.
- **Checkbox:** a 1rem paper square with a 4px radius and a 35% ink border, filled navy with a paper check when on. Its hit area extends 0.75rem sideways and 0.5rem vertically.

### Highlighter Mark (signature)
The swipe across an AI-picked subject, two translucent strokes with ragged ends that break per line. Full strength for "Now", the soft stroke for "Today". It draws once, left to right, when it first appears: 520ms, `cubic-bezier(0.16, 1, 0.3, 1)`. With reduced motion it simply appears. Text selection uses the same yellow and ink.

### Pen Note (signature)
Caveat 600 in pen blue at 1.15 leading, set directly under the thing it comments on with no box or quote marks. The AI writes the summary above its picks (1.35rem) and one reason under each pick (1.25rem); the app writes short margin notes in the same hand: "that's everything that matters" under the all-clear, beside the opened message in a conversation.

### Pen Tick and Strike (signature)
Dealt-with mail is ticked and struck through in pen. The tick takes the unread dot's place on the row (1rem; 0.875rem in a conversation) and draws once in 320ms with the same easing; with reduced motion it is static. The subject gets a 1.5px pen line-through and drops to muted ink. In the list, a done-green caption records what was done; a ticked message offers Not done when it was marked done by hand.

### List Rows
- **Inbox row:** described in Layout. A chosen row fills with paper tint; a row on its way to Trash drops to 50% opacity with a spinner in its delete button (150ms transitions).
- **AI pick row:** sender (semibold) and time or "Unread 3 days" (semibold ink when overdue), the highlighted subject, the pen reason, a caption of signals, and a Done button in the 7rem action column.
- **Waiting row:** mail a routine is waiting on, under a column head with a repeat icon: sender, date, plain subject, the routine line, Save and Done.

### Conversation
A chat on the sheet. Other people's messages sit left on paper tint; yours sit right on a 9% pen tint. Bubbles are 0.5rem round, padded 0.75rem by 1rem, at most 88% wide, with the sender and time in caption above and the message's own actions below. Attachments are small bordered paper chips with a paperclip, name and size. The reply box at the foot is a field-edge outlined panel with a ruled text area and a primary Send.

### Dialog
Paper, 0.5rem round, padded 1.5rem, up to 24rem wide, with the sheet shadow over a dimmed desk. A ghost close button sits top right; the footer is separated by a hairline rule and right-aligns its buttons on wide screens. Fades and scales in from 95% in 150ms.

### Status and Waiting
- **Working line:** a small spinner and muted body-small text saying what is happening; after 3 seconds it adds the elapsed seconds.
- **Progress bar:** 0.25rem tall, rounded, pen fill on a paper-tint track, width easing over 500ms.
- **Activity bar:** a 3px bar across the top of the window, shown only when a wait passes 400ms; a one-third segment slides across every 1.1s (`ease-in-out`). With reduced motion it holds still at full width and half opacity. Its segment is ballpoint blue: waiting is a pen state, and the highlighter stays reserved for mail.
- **Spinners** turn only under `motion-safe`.
- **Messages:** errors are red text, or a 0.5rem box with a 40% red border; neutral results are a hairline-bordered 0.5rem box.

### All Clear
When nothing is left, the sheet shows the cockatoo bird (7rem tall), one semibold line, and a pen note under it.

### Logo
The logo is the owner's file (`sc/Cockatoo_Logo.webp`), cropped, never redrawn. The rail (or the bar on narrow windows) carries the bird mark (`cockatoo-mark.png`, 2rem tall) with the lowercase wordmark in bold 1.125rem at -0.02em. The first-run setup screen leads with the full bird (`cockatoo-bird.webp`, 6rem tall), the wordmark at 1.875rem bold and the tagline "Only the emails that matter." in muted ink, above a hairline rule. The all-clear state reuses the bird. The foot of the rail prints "Only the emails that matter." and the build in muted caption. The name is always lowercase in the UI.

## Do's and Don'ts

### Do:
- **Do** put every screen on the one sheet: paper on the desk, 60rem wide (76rem for Clean up), edge to edge below 640px.
- **Do** keep the highlighter for AI-picked subjects only: the full stroke for "Now", `.soft` for "Today".
- **Do** write the AI's reasons and the app's margin notes in Caveat 600, pen blue, directly under what they comment on.
- **Do** mark dealt-with mail with the pen tick and a pen strike-through, and record what was done in a done-green caption.
- **Do** keep every action visible on its row, in the fixed 7rem action column on the right.
- **Do** separate rows and sections with hairline rules and column heads, not cards or boxes.
- **Do** let marks draw once (highlighter 520ms, tick 320ms, `cubic-bezier(0.16, 1, 0.3, 1)`) and switch every animation off under `prefers-reduced-motion`.
- **Do** use the ruled-line field (2px ink rule, pen on focus) for search and writing; the boxed field for forms.
- **Do** check both themes: every token has a dark value, and the highlighter's text turns light yellow in dark.

### Don't:
- **Don't** put the highlighter on headings, buttons, counts, routine mail or handled mail, or use the yellow as a fill or accent.
- **Don't** use Caveat for headings, buttons, fields, numbers or data, or in any colour but pen blue.
- **Don't** use pen blue as a button fill, link colour or decoration.
- **Don't** lift anything on the sheet with a shadow; the sheet and the dialog are the only lifted surfaces.
- **Don't** add menus, sub-menus, hover-only actions or context menus.
- **Don't** add a sidebar-list-reading-pane layout or a grey-and-blue mail-client palette.
- **Don't** use the destructive variant for moving mail to Trash; it is for removing cockatoo's own records.
- **Don't** animate anything beyond the first drawing of a mark and honest waiting indicators.
- **Don't** redraw, recolour or generate the cockatoo; use the crops of the owner's logo.
