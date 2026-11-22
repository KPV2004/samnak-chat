import { defineConfig } from "vitepress";

// The site is published under the landing page's Pages URL, at /samnak-chat/docs/.
const repo = "https://github.com/KPV2004/samnak-chat";
const landing = "https://kpv2004.github.io/samnak-chat/";

const enSidebar = [
  {
    text: "Start here",
    items: [
      { text: "Introduction", link: "/" },
      { text: "Install on your server", link: "/install" },
      { text: "Configuration", link: "/configuration" },
      { text: "Operate", link: "/operate" },
    ],
  },
  {
    text: "User guide",
    items: [
      { text: "Messenger", link: "/guide/messenger" },
      { text: "Tasks", link: "/guide/tasks" },
      { text: "Approvals", link: "/guide/approvals" },
      { text: "Helpdesk", link: "/guide/helpdesk" },
      { text: "Notifications", link: "/guide/notifications" },
      { text: "Workspace", link: "/guide/workspace" },
      { text: "Account", link: "/guide/account" },
    ],
  },
  {
    text: "Developers",
    items: [
      { text: "Development setup", link: "/developers/setup" },
      { text: "Architecture", link: "/developers/architecture" },
      { text: "Modules and rules", link: "/developers/modules" },
      { text: "API reference", link: "/developers/api" },
      { text: "Contributing", link: "/developers/contributing" },
      { text: "Decision records", link: "/developers/decisions" },
    ],
  },
  { text: "Changelog", link: "/changelog" },
];

const thSidebar = [
  {
    text: "เริ่มต้นที่นี่",
    items: [
      { text: "บทนำ", link: "/th/" },
      { text: "ติดตั้งบนเซิร์ฟเวอร์ของคุณ", link: "/th/install" },
      { text: "การตั้งค่า", link: "/th/configuration" },
      { text: "การดูแลระบบ", link: "/th/operate" },
    ],
  },
  {
    text: "คู่มือการใช้งาน",
    items: [
      { text: "แชท (Messenger)", link: "/th/guide/messenger" },
      { text: "งาน", link: "/th/guide/tasks" },
      { text: "คำขออนุมัติ", link: "/th/guide/approvals" },
      { text: "ศูนย์ช่วยเหลือ", link: "/th/guide/helpdesk" },
      { text: "การแจ้งเตือน", link: "/th/guide/notifications" },
      { text: "เวิร์กสเปซ", link: "/th/guide/workspace" },
      { text: "บัญชี", link: "/th/guide/account" },
    ],
  },
  {
    text: "สำหรับนักพัฒนา",
    items: [
      { text: "ตั้งค่าสำหรับพัฒนา", link: "/th/developers/setup" },
      { text: "สถาปัตยกรรม", link: "/th/developers/architecture" },
      { text: "โมดูลและกฎ", link: "/th/developers/modules" },
      { text: "อ้างอิง API", link: "/th/developers/api" },
      { text: "การมีส่วนร่วม", link: "/th/developers/contributing" },
      { text: "บันทึกการตัดสินใจ", link: "/th/developers/decisions" },
    ],
  },
  { text: "บันทึกการเปลี่ยนแปลง", link: "/th/changelog" },
];

export default defineConfig({
  base: "/samnak-chat/docs/",
  cleanUrls: false,
  lastUpdated: false,
  srcExclude: ["README.md"],
  head: [
    ["link", { rel: "icon", href: "/samnak-chat/docs/favicon.svg", type: "image/svg+xml" }],
    ["meta", { name: "theme-color", content: "#1f6b3a" }],
  ],

  locales: {
    root: {
      label: "English",
      lang: "en-US",
      title: "Samnak Chat docs",
      description: "Install, configure, operate and develop Samnak Chat, the self-hosted team chat with tasks, approvals and a helpdesk.",
      themeConfig: {
        nav: [
          { text: "Install", link: "/install" },
          { text: "User guide", link: "/guide/messenger" },
          { text: "Developers", link: "/developers/setup" },
          { text: "Changelog", link: "/changelog" },
          { text: "Landing page", link: landing },
        ],
        sidebar: enSidebar,
        outline: { level: [2, 3], label: "On this page" },
        docFooter: { prev: "Previous page", next: "Next page" },
        footer: {
          message: "Open source under AGPL-3.0.",
          copyright: "© 2026 Samnak Chat",
        },
      },
    },
    th: {
      label: "ไทย",
      lang: "th-TH",
      link: "/th/",
      title: "เอกสาร Samnak Chat",
      description: "ติดตั้ง ตั้งค่า ดูแล และพัฒนา Samnak Chat แชทสำหรับทีมแบบติดตั้งเอง พร้อมงาน การอนุมัติ และศูนย์ช่วยเหลือ",
      themeConfig: {
        nav: [
          { text: "ติดตั้ง", link: "/th/install" },
          { text: "คู่มือการใช้งาน", link: "/th/guide/messenger" },
          { text: "สำหรับนักพัฒนา", link: "/th/developers/setup" },
          { text: "บันทึกการเปลี่ยนแปลง", link: "/th/changelog" },
          { text: "หน้าแรกโครงการ", link: landing + "th/" },
        ],
        sidebar: thSidebar,
        outline: { level: [2, 3], label: "ในหน้านี้" },
        docFooter: { prev: "หน้าก่อนหน้า", next: "หน้าถัดไป" },
        sidebarMenuLabel: "เมนู",
        returnToTopLabel: "กลับขึ้นด้านบน",
        darkModeSwitchLabel: "ธีม",
        lightModeSwitchTitle: "สลับเป็นโหมดสว่าง",
        darkModeSwitchTitle: "สลับเป็นโหมดมืด",
        skipToContentLabel: "ข้ามไปยังเนื้อหา",
        langMenuLabel: "เปลี่ยนภาษา",
        footer: {
          message: "เผยแพร่ภายใต้ใบอนุญาต AGPL-3.0",
          copyright: "© 2026 Samnak Chat",
        },
      },
    },
  },

  themeConfig: {
    socialLinks: [{ icon: "github", link: repo }],
    search: {
      provider: "local",
      options: {
        locales: {
          th: {
            translations: {
              button: { buttonText: "ค้นหา", buttonAriaLabel: "ค้นหา" },
              modal: {
                noResultsText: "ไม่พบผลลัพธ์สำหรับ",
                resetButtonTitle: "ล้างการค้นหา",
                footer: {
                  selectText: "เลือก",
                  navigateText: "เลื่อน",
                  closeText: "ปิด",
                },
              },
            },
          },
        },
      },
    },
  },
});
