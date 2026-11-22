// Writes the pages that come from files in this repository, before VitePress
// runs. Nothing here is copied by hand: the API reference is built from
// apps/server/api/openapi.yaml and the changelog from CHANGELOG.md. The outputs
// are git-ignored (see ../.gitignore).
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "yaml";

const docsRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = resolve(docsRoot, "../..");
const repoBlob = "https://github.com/KPV2004/samnak-chat/blob/main/";

// Groups of the API reference, in the order the page shows them. An operation
// with a tag not listed here still appears, at the end.
const moduleOrder = ["auth", "workspace", "chat", "notify", "task", "approval", "helpdesk", "meta"];
const methods = ["get", "post", "put", "patch", "delete"];

const apiText = {
  en: {
    title: "API reference",
    description: "Every endpoint of the Samnak Chat API, generated from the OpenAPI file.",
    intro: [
      "This page is generated when the site is built, from `apps/server/api/openapi.yaml`. Every endpoint below is in that file; nothing is copied by hand. The summaries come from the file and are in English.",
      "All paths are under `/api/v1`. The web app calls the same API, so the endpoints here are the ones it uses.",
      "Calls about one workspace send its ID in the `X-Workspace-ID` header. The server checks that you are a member and takes the workspace from that header, never from the request body. The header is not in the OpenAPI file.",
    ],
    signIn: "Sign-in",
    bearer: "Access token",
    none: "Not needed",
    success: "Success",
    method: "Method",
    path: "Path",
    summary: "Summary",
    errors: [
      "**Errors** have one shape: `{ \"error\": { \"code\": \"not_found\", \"message\": \"channel not found\" } }`. Read `code` in your code, and show `message` to people only as a hint.",
      "**Access tokens** last 15 minutes. Send one as `Authorization: Bearer <token>`. Refresh it with `POST /auth/refresh`; the refresh token rotates on every use.",
      "**Rate limits** answer 429 with a `Retry-After` header when a client is over its limit. Signed-in users are counted by user, signed-out clients by IP address (see [Configuration](/configuration)).",
      `The OpenAPI file is at [apps/server/api/openapi.yaml](${repoBlob}apps/server/api/openapi.yaml) in the repository.`,
    ],
    moduleNames: {
      auth: "Accounts (auth)",
      workspace: "Workspaces (workspace)",
      chat: "Chat (chat)",
      notify: "Notifications (notify)",
      task: "Tasks (task)",
      approval: "Approvals (approval)",
      helpdesk: "Helpdesk (helpdesk)",
      meta: "Server information (meta)",
    },
  },
  th: {
    title: "อ้างอิง API",
    description: "ทุก endpoint ของ API ของ Samnak Chat สร้างจากไฟล์ OpenAPI",
    intro: [
      "หน้านี้สร้างขึ้นตอนสร้างเว็บไซต์ จากไฟล์ `apps/server/api/openapi.yaml` ทุก endpoint ด้านล่างอยู่ในไฟล์นั้น ไม่ได้คัดลอกด้วยมือ คำอธิบายสั้นมาจากไฟล์และเป็นภาษาอังกฤษ",
      "เส้นทางทั้งหมดอยู่ใต้ `/api/v1` เว็บแอปเรียก API เดียวกันนี้ จึง endpoint ที่แสดงที่นี่คือ endpoint ที่เว็บแอปใช้",
      "การเรียกที่เกี่ยวกับเวิร์กสเปซหนึ่ง ส่ง ID ของเวิร์กสเปซนั้นในหัว `X-Workspace-ID` เซิร์ฟเวอร์ตรวจว่าคุณเป็นสมาชิก และรับเวิร์กสเปซจากหัวนี้ ไม่รับจากเนื้อหาคำขอ หัวนี้ไม่มีอยู่ในไฟล์ OpenAPI",
    ],
    signIn: "ต้องเข้าสู่ระบบ",
    bearer: "ใช้ access token",
    none: "ไม่ต้อง",
    success: "สำเร็จ",
    method: "เมธอด",
    path: "เส้นทาง",
    summary: "คำอธิบาย",
    errors: [
      "**ข้อผิดพลาด** มีรูปแบบเดียว: `{ \"error\": { \"code\": \"not_found\", \"message\": \"channel not found\" } }` ให้โค้ดของคุณอ่านค่า `code` และแสดง `message` ให้ผู้ใช้เป็นคำใบ้เท่านั้น",
      "**Access token** มีอายุ 15 นาที ส่งด้วย `Authorization: Bearer <token>` ขอใหม่ด้วย `POST /auth/refresh` refresh token จะหมุนเปลี่ยนทุกครั้งที่ใช้",
      "**ขีดจำกัดการเรียก** ตอบ 429 พร้อมหัว `Retry-After` เมื่อไคลเอนต์เกินขีดจำกัด ผู้ใช้ที่เข้าสู่ระบบนับตามผู้ใช้ ไคลเอนต์ที่ยังไม่เข้าสู่ระบบนับตาม IP (ดู [การตั้งค่า](/th/configuration))",
      `ไฟล์ OpenAPI อยู่ที่ [apps/server/api/openapi.yaml](${repoBlob}apps/server/api/openapi.yaml) ในที่เก็บโค้ด`,
    ],
    moduleNames: {
      auth: "บัญชี (auth)",
      workspace: "เวิร์กสเปซ (workspace)",
      chat: "แชท (chat)",
      notify: "การแจ้งเตือน (notify)",
      task: "งาน (task)",
      approval: "คำขออนุมัติ (approval)",
      helpdesk: "ศูนย์ช่วยเหลือ (helpdesk)",
      meta: "ข้อมูลเซิร์ฟเวอร์ (meta)",
    },
  },
};

function cell(text) {
  return String(text ?? "").replace(/\|/g, "\\|").replace(/\s+/g, " ").trim();
}

function operationsByModule(spec) {
  const groups = new Map();
  for (const [path, item] of Object.entries(spec.paths ?? {})) {
    for (const method of methods) {
      const op = item[method];
      if (!op) continue;
      const tag = op.tags?.[0] ?? "meta";
      const success = Object.keys(op.responses ?? {}).find((code) => code.startsWith("2")) ?? "";
      const row = {
        method: method.toUpperCase(),
        path: `/api/v1${path}`,
        summary: op.summary ?? "",
        bearer: (op.security ?? []).some((s) => "bearer" in s),
        success,
      };
      if (!groups.has(tag)) groups.set(tag, []);
      groups.get(tag).push(row);
    }
  }
  const rank = (tag) => {
    const i = moduleOrder.indexOf(tag);
    return i === -1 ? moduleOrder.length : i;
  };
  return [...groups.entries()].sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b));
}

function apiPage(lang, groups) {
  const t = apiText[lang];
  const lines = [
    "---",
    `title: ${t.title}`,
    `description: ${t.description}`,
    "---",
    "",
    "<!-- Generated by apps/docs/scripts/generate.mjs from apps/server/api/openapi.yaml. Do not edit. -->",
    "",
    `# ${t.title}`,
    "",
    ...t.intro.flatMap((p) => [p, ""]),
    ...t.errors.flatMap((p) => [`- ${p}`]),
    "",
  ];
  for (const [tag, rows] of groups) {
    lines.push(`## ${t.moduleNames[tag] ?? tag}`, "");
    lines.push(`| ${t.method} | ${t.path} | ${t.summary} | ${t.signIn} | ${t.success} |`);
    lines.push("| --- | --- | --- | --- | --- |");
    for (const r of rows) {
      lines.push(
        `| ${r.method} | \`${cell(r.path)}\` | ${cell(r.summary)} | ${r.bearer ? t.bearer : t.none} | ${r.success} |`,
      );
    }
    lines.push("");
  }
  return { file: lang === "th" ? "th/developers/api.md" : "developers/api.md", text: lines.join("\n") + "\n" };
}

function changelogPage(lang, source) {
  // The file's own title and intro are dropped; the page has its own. The
  // relative links in the file point at other repository files, so they go to
  // GitHub, which keeps them working on the site.
  const body = source
    .slice(source.indexOf("\n## "))
    .replace(/\]\((?![a-z]+:|#)([^)]+)\)/g, `](${repoBlob}$1)`);
  const intro =
    lang === "th"
      ? [
          "การเปลี่ยนแปลงทุกรุ่นของ Samnak Chat ตามรูปแบบ Keep a Changelog ข้อความด้านล่างเป็นภาษาอังกฤษ เพราะไฟล์ต้นทางเขียนเป็นภาษาอังกฤษ",
          "",
        ]
      : [
          "Every change to Samnak Chat, by release. The file in the repository is [CHANGELOG.md](https://github.com/KPV2004/samnak-chat/blob/main/CHANGELOG.md).",
          "",
        ];
  const title = lang === "th" ? "บันทึกการเปลี่ยนแปลง" : "Changelog";
  const description = lang === "th" ? "การเปลี่ยนแปลงของ Samnak Chat ในแต่ละรุ่น" : "Changes to Samnak Chat in each release.";
  return {
    file: lang === "th" ? "th/changelog.md" : "changelog.md",
    text: [
      "---",
      `title: ${title}`,
      `description: ${description}`,
      "---",
      "",
      "<!-- Generated by apps/docs/scripts/generate.mjs from CHANGELOG.md. Do not edit. -->",
      "",
      `# ${title}`,
      "",
      ...intro,
      body.trimStart(),
    ].join("\n"),
  };
}

async function write(page) {
  const path = join(docsRoot, page.file);
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, page.text);
  console.log(`wrote ${page.file}`);
}

const spec = parse(await readFile(join(repoRoot, "apps/server/api/openapi.yaml"), "utf8"));
const changelog = await readFile(join(repoRoot, "CHANGELOG.md"), "utf8");
const groups = operationsByModule(spec);

for (const lang of ["en", "th"]) {
  await write(apiPage(lang, groups));
  await write(changelogPage(lang, changelog));
}
