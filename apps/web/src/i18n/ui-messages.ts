// Strings of the app frame and the shared UI components (theme, menus, close
// buttons). messages.ts spreads these into each language, and i18n.test.ts
// checks both have the same keys.
export const uiEn = {
  "ui.close": "Close",
  "ui.skipToContent": "Skip to content",
  "ui.openMenu": "Open menu",
  "ui.account": "Account",
  "theme.label": "Theme",
  "theme.light": "Light",
  "theme.dark": "Dark",
  "theme.system": "System",
} as const;

export const uiTh = {
  "ui.close": "ปิด",
  "ui.skipToContent": "ข้ามไปยังเนื้อหา",
  "ui.openMenu": "เปิดเมนู",
  "ui.account": "บัญชี",
  "theme.label": "ธีม",
  "theme.light": "สว่าง",
  "theme.dark": "มืด",
  "theme.system": "ตามระบบ",
} as const;
