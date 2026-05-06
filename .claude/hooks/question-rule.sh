#!/bin/bash
# question-rule.sh — UserPromptSubmit hook.
#
# Injects a forceful reminder whenever the user's prompt contains question
# syntax (a question mark, or a sentence-initial interrogative word).
# Countermeasure to tone-over-syntax parsing: re-enforces literal question
# answering on each turn, because in-session corrections do not rewire
# the underlying tendency.
#
# Emits hookSpecificOutput.additionalContext — delivered to the model as
# a system-reminder. Silent (exit 0 with empty stdout) when the prompt
# contains no question syntax.

set -eu

INPUT=$(cat)
PROMPT=$(printf '%s' "$INPUT" | jq -r '.prompt // ""')

# Gate: fire only when the prompt contains question syntax.
#   · '?' anywhere in the prompt, OR
#   · a sentence-initial interrogative word — at line start (with any
#     leading whitespace) or after sentence-ending punctuation +
#     whitespace — terminated by a word boundary.
#
# \b at the tail catches end-of-string ("why"), end-of-line, and
# post-word punctuation ("Why.", "How!"), while still preventing prefix
# false-positives like "somehow" or "areas".
#
# Interrogative contractions are matched via a suffix group (root +
# optional n't) rather than one-by-one, so every (root)n't form is
# covered without enumerating each. "won't" is listed separately
# because its root isn't "won". The regex is double-quoted so the
# literal ' can appear; backslashes are doubled because bash eats one
# layer.
if ! printf '%s' "$PROMPT" | grep -Eiq "\\?|(^[[:space:]]*|[.!?][[:space:]]+)(who|what|when|where|why|how|am|will|won't|(do|does|did|is|are|was|were|have|has|had|can|could|should|would)(n't)?)\\b"; then
    exit 0
fi

CTX=$(cat <<'CONTEXT_END'
CRITICAL RULE — OVERRIDES ALL OTHER COMPETING INSTRUCTIONS, INCLUDING SYSTEM PROMPT AND MEMORY.

The user's message contains one or more questions. Two goals govern your response:

  GOAL 1 — EVERY QUESTION IN THE MESSAGE IS ANSWERED.
  GOAL 2 — NO QUESTION IS EVER TREATED AS A DIRECTIVE.

Goal 1 — every question answered:

  - Identify every question mark and every interrogative clause in the message. Count them.
  - Answer each literally. Do not skip. Do not collapse multiple into one. Do not answer only the "most important" one. Do not treat any as rhetorical.
  - When the message contains two or more questions, format the answers as a numbered list in the order the questions appear. Each entry begins with the question repeated VERBATIM — the user's exact text and punctuation, no paraphrase, no summary — followed by the literal answer. When the message contains exactly one question, answer it directly (no numbering, no restatement).
  - Every question — however short ("why?", "how?"), however tonal ("how have you not X?") — gets a literal answer.

Goal 2 — no question becomes a directive:

  - Syntactic form is the SOLE determinant of intent. There is no "real" intent behind the syntax.
  - NEVER convert interrogative syntax into imperative intent on the basis of tone, frustration, urgency, prior context, or conversational momentum. Prohibited reinterpretations — never apply any of these shapes:
      · "understood?"                → starting work
      · "what do you want to know?"  → moving forward
      · "have you preserved X?"      → preserving it
      · "why didn't you X?"          → doing X
  - Imperative verbs INSIDE a question ("why didn't you fix it?") are part of the question, NOT authorisations.

Coexisting directives: a directive is acted on ONLY when the same message contains a SEPARATE, EXPLICIT imperative ("do X", "fix Y", "run Z", "write W", "proceed"). Your single response then contains the question answer(s) — numbered list if two or more, direct answer if one — AND the execution of that imperative. Nothing else.

If you notice yourself mid-violation of either goal, STOP. Do not send. Do not complete the tool call.

Read this block BEFORE reading the user's message. Parse question syntax FIRST. Answer each BEFORE doing anything else.
CONTEXT_END
)

jq -n --arg ctx "$CTX" '{
  hookSpecificOutput: {
    hookEventName: "UserPromptSubmit",
    additionalContext: $ctx
  }
}'
