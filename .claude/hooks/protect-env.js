#!/usr/bin/env node

// .env holds a real credential; .env.example (the checked-in template) does
// not and must stay readable. Matches the exact filename ".env" — a leading
// path segment is fine, but anything appended after ".env" (".env.example",
// ".env.local") is a different, unprotected file.
function isEnvFile(p) {
  return typeof p === "string" && /(^|\/)\.env$/.test(p);
}

function block(msg) {
  process.stderr.write(`Blocked: ${msg}\n`);
  process.exit(2);
}

let raw = "";
process.stdin.on("data", (chunk) => (raw += chunk));
process.stdin.on("end", () => {
  let input;
  try {
    input = JSON.parse(raw);
  } catch {
    process.exit(0);
  }

  const toolName = input?.tool_name;

  if (toolName === "Read") {
    const filePath = input?.tool_input?.file_path;
    if (isEnvFile(filePath)) {
      block(
        "\".env\" contains a real credential and is protected from being read by this tool. If you need a value from it, ask the user directly."
      );
    }
  }

  if (toolName === "Bash") {
    const command = input?.tool_input?.command;
    if (typeof command === "string") {
      // Tokenize on whitespace/quotes (matches protect-tests.js's approach)
      // rather than matching a verb list (cat/head/tail/...) — a blanket
      // block on any command that references the literal ".env" token is
      // harder to route around than trying to enumerate every way a shell
      // command could read a file's contents.
      const tokens = command.match(/[^\s"']+/g) || [];
      if (tokens.some(isEnvFile)) {
        block(
          "command references \".env\", which contains a real credential and is protected from this tool. If you need a value from it, ask the user directly."
        );
      }
    }
  }

  process.exit(0);
});
