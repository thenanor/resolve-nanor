#!/usr/bin/env node

let raw = "";
process.stdin.on("data", (chunk) => (raw += chunk));
process.stdin.on("end", () => {
  let input;
  try {
    input = JSON.parse(raw);
  } catch {
    process.exit(0);
  }

  const filePath = input?.tool_input?.file_path;
  if (typeof filePath === "string" && /audit/i.test(filePath)) {
    process.stderr.write(
      "Blocked: the audit service is protected and cannot be modified by this tool.\n"
    );
    process.exit(2);
  }

  process.exit(0);
});
