# Claude Journal

Working memory for Claude sessions on AgentWarden. Updated each session so context carries over.

## Structure

```
.claude-journal/
  README.md                    ← This file
  PM.md                        ← Product management: push rules, competitive positioning, market research
  competitive-analysis.md      ← Deep dive on 30+ competitors across 5 categories
  tracker.csv                  ← Daily task log (spreadsheet-style)
  future-todos.md              ← Prioritized backlog & ideas
  learnings.md                 ← Architecture decisions & tradeoffs
  sessions/
    2025-02-24.md              ← What was done, key fixes, where we left off
    2025-02-25.md              ← ...
```

## How It Works
- **Start of session**: Read the latest `sessions/YYYY-MM-DD.md` to pick up context
- **During session**: Update `tracker.csv` with tasks completed
- **End of session**: Write a new session log with what was done, blockers, and what next session should do
- **Ongoing**: Update `future-todos.md` and `learnings.md` as decisions are made
- **Before each push**: Check `PM.md` push review rules and quality checklist
