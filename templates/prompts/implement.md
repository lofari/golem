You are implementing a code change in a software project.

# Goal
${goal}

# Plan
${plan}

# Instructions
- Write the code changes needed to accomplish the goal
- Write or update tests for your changes
- When finished, write a session-output.json file in the working directory

Write session-output.json containing:
{"test-results": {"status": "pass|fail", "summary": "..."}}

Note: Do NOT write a "code" key — the engine detects changed files automatically via git diff.

## Previous Error Context
${_error_context}
