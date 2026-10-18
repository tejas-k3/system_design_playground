# Accessibility testing

What is checked automatically, and the script for the checks that need a real screen reader.

## What CI already checks (not a substitute for a screen reader)

`make browser-check` (the `browser` CI job) drives headless Chrome and reads its accessibility tree:

- no interactive control without an accessible name, on the welcome screen, with an example loaded, while it runs, and with the help dialog open;
- a live region (`status`, `alert` or `log`) is exposed while a simulation runs;
- focus moves inside a dialog when it opens;
- the layout stays usable at five laptop sizes, and nothing is left behind when a component is dragged.

These confirm the tree is well formed. They cannot tell whether what is announced makes sense, how it sounds, or whether the order is natural to someone who cannot see the screen. Only a person with a screen reader can.

## Results so far

| Date | Tester | Screen reader + browser | OS | Scripts run | Result |
|---|---|---|---|---|---|
| _none yet_ | | | | | |

**No screen-reader pass has been recorded. Do not read the automated checks above as one.** Add a row for each pass; record anything that failed as a GitHub issue and link it in the Notes column of the results template below.

## Set-up

| | VoiceOver | NVDA |
|---|---|---|
| OS | macOS | Windows 10 or 11 |
| Browser | Safari, then Chrome | Firefox, then Chrome |
| Start | Cmd+F5 | Ctrl+Alt+N |
| Basic keys | VO = Ctrl+Option; VO+Right moves; VO+Space activates; VO+U opens the rotor | Insert is the NVDA key; Down arrow reads on; Enter activates; NVDA+F7 lists elements |

Start the app (`make up`, then open http://localhost:8090) in a fresh window with the screen reader already running. Turn off speech after the pass, not before: read what it says, do not read the screen.

## Script

For each step, note what was announced and compare it with "Expected". A step passes if everything in Expected was heard (wording may differ) and nothing confusing or wrong was added.

### 1. Landing

1. Load the page. Expected: the page title, then a banner and the welcome heading "Sketch a system. Send traffic. Watch it break.".
2. Press Tab once. Expected: the first stop is the link "Skip to the canvas". Press Enter; focus moves to the canvas area.
3. Press Shift+Tab to leave the canvas and keep tabbing through the toolbar. Expected: every button is announced by name (Select tool, Connect tool, Undo, Redo, Fit, Play, Step, Reset, Simulation speed, Challenges, ...), never as "button" alone.

### 2. Build a design by keyboard

1. Tab to the component library. Expected: groups named Traffic, Compute, Data, Async and external.
2. Activate "Users", then "App Server", then "Database" with Enter. Expected: each is added; the outline list of components (a list of the components on the canvas) reflects them.
3. Move through the outline list; select "Users". Expected: the inspector opens and its heading names the component; its settings are labelled form controls.
4. In the inspector, find the connect control and connect Users to App Server, then App Server to Database. Expected: confirmation of the connection (toast read as a status), and it appears in the outline.

### 3. Run and listen

1. Press Play (Space). Expected: "Simulation running."
2. Wait about ten seconds without touching anything. Expected: a one-sentence summary such as "1,453 requests, 100.0% succeeded, median 127 ms, 95th percentile 161 ms, no failures." and again every ten seconds. It must not be announced more often, and it must not interrupt typing.
3. Pause. Expected: "Paused." and the summary.
4. Reach the metric tiles and the three charts. Expected: each chart is an image with a text alternative that includes its latest values; the event list under the charts is readable.

### 4. A challenge

1. Open Challenges; the dialog gets focus and is announced as a dialog; Escape closes it and focus returns to the button.
2. Open "Hello, database". Expected: the challenge heading, the goals as a list with their current values, and the prediction question with its options as a group of buttons.
3. Answer the prediction. Expected: the explanation is read.
4. Reveal a hint (button "Give me a hint"). Expected: the new hint is reachable immediately after the button.
5. Run until the result appears. Expected: the result dialog is announced with the stars earned and whether it passed.

### 5. After the run

1. In the metrics area open "Request traces" and choose a failed request. Expected: its steps are readable in order, with the diagnosis sentence.
2. Open the design review in the overview. Expected: findings are announced with the words Error, Warning or Note, followed by the fix.
3. Open "Capacity curve". Expected: a dialog; the "Run the sweep" button; afterwards the sentence about the knee and a table with a caption.

## Results template

Copy this per pass.

```
Date:
Tester:
Screen reader + version:
Browser + version:
OS:
Build (commit):

Step  Pass/Fail  What was heard (if different)  Issue link
1.1
1.2
...
Overall notes:
```

## When something fails

File an issue titled `a11y: <step> <short description>` with the screen reader, browser, build and exactly what was heard. Fix it, re-run the step, and add the new row to the results table. Where the problem can be caught automatically (a missing name, a missing live region), also add it to `docs/review/tools/ax-audit.mjs` so CI keeps it fixed.
