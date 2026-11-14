export const messages = {
  th: {
    title: "Samnak Chat",
    modules: "โมดูลที่เปิดใช้งาน",
    loading: "กำลังโหลด…",
    error: "โหลดข้อมูลเซิร์ฟเวอร์ไม่สำเร็จ",
    retry: "ลองอีกครั้ง",
    noModules: "ยังไม่มีโมดูลที่เปิดใช้งาน",
    language: "ภาษา",
  },
  en: {
    title: "Samnak Chat",
    modules: "Enabled modules",
    loading: "Loading…",
    error: "Could not load server info",
    retry: "Retry",
    noModules: "No modules enabled yet",
    language: "Language",
  },
} as const;

export type Locale = keyof typeof messages;
export type MessageKey = keyof (typeof messages)["en"];
