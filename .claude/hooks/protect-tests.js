#!/usr/bin/env node

const fs = require("fs");

const TEST_FILE_RE = /(_test\.go|\.(?:test|spec)\.[jt]sx?)$/i;
// same suffixes, but usable mid-string to scan Bash command arguments
const TEST_FILE_TOKEN_RE = /[^\s"']*(?:_test\.go|\.(?:test|spec)\.[jt]sx?)\b/gi;

function escapeRegex(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function block(msg) {
  process.stderr.write(`Blocked: ${msg}\n`);
  process.exit(2);
}

// An edit counts as pure addition only if everything that was already
// there survives untouched — old_string must appear verbatim inside
// new_string. That allows inserting a new test case (old_string as an
// anchor, new_string = anchor + new content) while still blocking any
// edit that removes or rewrites existing text.
function isAdditiveEdit(oldString, newString) {
  return (
    typeof oldString === "string" &&
    typeof newString === "string" &&
    newString.includes(oldString)
  );
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

  if (toolName === "Edit") {
    const filePath = input?.tool_input?.file_path;
    if (typeof filePath === "string" && TEST_FILE_RE.test(filePath)) {
      const { old_string, new_string } = input?.tool_input ?? {};
      if (!isAdditiveEdit(old_string, new_string)) {
        block(
          `"${filePath}" is an existing test file. Deleting or modifying existing test content is not allowed — add a new test instead of changing this one, or ask the user to make the change themselves.`
        );
      }
    }
  }

  if (toolName === "MultiEdit") {
    const filePath = input?.tool_input?.file_path;
    const edits = input?.tool_input?.edits;
    if (
      typeof filePath === "string" &&
      TEST_FILE_RE.test(filePath) &&
      Array.isArray(edits)
    ) {
      const hasNonAdditiveEdit = edits.some(
        (e) => !isAdditiveEdit(e?.old_string, e?.new_string)
      );
      if (hasNonAdditiveEdit) {
        block(
          `"${filePath}" is an existing test file. Deleting or modifying existing test content is not allowed — add a new test instead of changing this one, or ask the user to make the change themselves.`
        );
      }
    }
  }

  if (toolName === "Write") {
    const filePath = input?.tool_input?.file_path;
    if (
      typeof filePath === "string" &&
      TEST_FILE_RE.test(filePath) &&
      fs.existsSync(filePath)
    ) {
      const newContent = input?.tool_input?.content;
      let existingContent = null;
      try {
        existingContent = fs.readFileSync(filePath, "utf8");
      } catch {
        // unreadable existing file — fall through to block below
      }
      if (
        typeof newContent !== "string" ||
        existingContent === null ||
        !newContent.includes(existingContent)
      ) {
        block(
          `"${filePath}" is an existing test file. Overwriting it in a way that removes or changes existing content is not allowed — create a new test file instead, or ask the user to make the change themselves.`
        );
      }
    }
  }

  if (toolName === "Bash") {
    const command = input?.tool_input?.command;
    if (typeof command === "string") {
      const hits = [...new Set(command.match(TEST_FILE_TOKEN_RE) || [])];
      if (hits.length > 0) {
        const isDelete = /\b(rm|unlink|git\s+rm|shred)\b/i.test(command);
        // Only a truncating single `>` destroys existing content; `>>`
        // append cannot remove what's already in the file, so it's left
        // alone to support appending a new test case via shell.
        const isOverwrite = hits.some((hit) =>
          new RegExp(`(?<!>)>(?!>)\\s*${escapeRegex(hit)}\\b`).test(command)
        );
        if (isDelete || isOverwrite) {
          block(
            `command targets existing test file(s) (${hits.join(
              ", "
            )}) with a delete or truncating-overwrite operation (rm, git rm, unlink, shred, or \`>\` redirection). Deleting or overwriting existing tests via shell is not allowed.`
          );
        }
      }
    }
  }

  process.exit(0);
});
