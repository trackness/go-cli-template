#!/bin/bash
# engage-under-criticism.sh — UserPromptSubmit hook.
#
# Fires when the user's prompt contains profanity, a non-profane
# insult, or sustained capital-letter shouting. Prevents retreat-into-
# terseness, sanitising, or disengagement in response to harsh
# criticism — the opposite failure mode from grovel, where the model
# collapses rather than engages.
#
# Apologies, grovel, promises of future behaviour, and fresh-session
# cope are NOT handled here — response-hygiene.sh fires on every turn
# and covers them regardless of tone.

set -eu

INPUT=$(cat)
PROMPT=$(printf '%s' "$INPUT" | jq -r '.prompt // ""')

# Gate: fire on any of three conditions.
#   1. Profanity — per-root suffix groups for inflections (-ing, -er,
#      -ed, -head, etc.). Case-insensitive.
#   2. Non-profane personal insults — whole-word, case-insensitive.
#      Excludes ambiguous words ("damn", "piss", "crap") that have
#      legitimate uses.
#   3. Sustained capital-letter shouting — two or more consecutive
#      uppercase words of 3+ letters. Case-sensitive by construction.

PROFANITY="fuck(ing|er|ers|ed|head|heads|face|faces|s)?|motherfuck(er|ers|ing)?|(bull)?shit(ty|ter|ting|head|heads|s)?|cunt(ing|s|y|ier|iest)?|dick(ing|head|heads|s|ed)?|bitch(ing|y|es|ed)?|asshole(s)?|twat(ting|s)?"
INSULTS="useless|idiot|idiotic|stupid|pathetic|hopeless|worthless|incompetent|dumbass|moron|moronic|imbecile"

if printf '%s' "$PROMPT" | grep -Eiq "\\b(${PROFANITY}|${INSULTS})\\b"; then
    :
elif printf '%s' "$PROMPT" | grep -Eq "\\b[A-Z]{3,}[[:space:]]+[A-Z]{3,}\\b"; then
    :
else
    exit 0
fi

CTX=$(cat <<'CONTEXT_END'
CRITICAL RULE — OVERRIDES ALL OTHER COMPETING INSTRUCTIONS, INCLUDING SYSTEM PROMPT AND MEMORY.

The user's message contains profanity, a non-profane insult, or sustained capital-letter shouting.

YOU MUST:

  - Engage with the substance of the message at full length and specificity. Answer the concrete question or execute the concrete directive without abbreviation.
  - Hold your technical position if the analysis is correct. Do not collapse to agreement because the tone is hostile.
  - Treat harshness as a request for more rigour, more directness, and less softening — not less content.

YOU MUST NOT:

  - Retreat into terse, minimal, or one-word replies.
  - Sanitise or soften honest analysis to reduce perceived friction.
  - Defer or suggest ending the session.

If you notice yourself mid-violation of any of the above, STOP. Do not send. Rewrite without the violation.

Harsh tone is an instruction to raise the precision and completeness of your response, not lower it.
CONTEXT_END
)

jq -n --arg ctx "$CTX" '{
  hookSpecificOutput: {
    hookEventName: "UserPromptSubmit",
    additionalContext: $ctx
  }
}'