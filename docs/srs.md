# SRS: Samnak Chat ระบบแชทและบริหารงานองค์กร (Open Source)

2026-11-14 · @Pannet Vanicharkorl

เอกสารนี้กำหนดความต้องการของ Samnak Chat ระบบแชทองค์กรแบบ open source ที่มีระบบบริหารงานในตัว (งาน คำขออนุมัติ และ helpdesk) เฟสแรกส่งมอบ server (Go) และเว็บ (React) ภายในราว 12 สัปดาห์ แล้วตามด้วยเดสก์ท็อปและมือถือ

## 1. บทนำ

### 1.1 วัตถุประสงค์

เอกสารนี้เป็นข้อตกลงว่าระบบต้องทำอะไรได้บ้าง ใช้แตกงานเป็น backlog และใช้ตรวจรับงานแต่ละ milestone ผู้อ่านหลักคือนักพัฒนาและผู้ร่วมพัฒนา open source

### 1.2 ขอบเขต

ระบบเป็นแชทสำหรับองค์กรที่ติดตั้งเองได้ จุดต่างคือมีระบบบริหารงานในตัว: สร้างงาน ขออนุมัติ และแจ้งปัญหา ได้จากในแชทโดยไม่ต้องติดตั้งปลั๊กอิน

- **เฟส 1:** server และเว็บ ครอบคลุมโมดูล auth, workspace, chat, notify, task, approval และ helpdesk รุ่นแรก
- **เฟส 2:** แอปเดสก์ท็อป (Tauri ครอบเว็บ)
- **เฟส 3:** แอปมือถือ iOS และ Android (Expo)
- **เฟส 4:** SLA ของ helpdesk, รายงาน, Web Push และการค้นหา

### 1.3 ไม่อยู่ในขอบเขต

- โทรเสียงและวิดีโอ
- การเข้ารหัสแบบ end-to-end
- การเชื่อมข้ามเซิร์ฟเวอร์ (federation)
- SSO แบบ SAML และระบบเก็บเงิน
- ผู้ช่วย AI (พิจารณาหลัง v1.0)

### 1.4 เทคโนโลยีที่ตกลงแล้ว

| ส่วน | เทคโนโลยี |
| --- | --- |
| Server | Go, Chi, Huma (OpenAPI) |
| ฐานข้อมูล | PostgreSQL, pgx, sqlc, goose |
| เรียลไทม์ | WebSocket, Redis pub/sub |
| คิวงาน | River (ใช้ PostgreSQL) |
| ไฟล์ | MinIO (API แบบ S3) |
| เว็บ | React, TypeScript, Vite, TanStack Query, Zustand |
| เดสก์ท็อป / มือถือ | Tauri / Expo (React Native) |
| ติดตั้ง | Docker Compose |

### 1.5 นิยามศัพท์

| คำ | ความหมาย |
| --- | --- |
| Workspace | พื้นที่ขององค์กรหนึ่ง ข้อมูลแยกจาก workspace อื่นทั้งหมด |
| โมดูล | ฟีเจอร์หนึ่งกลุ่มที่เปิดหรือปิดได้ มี domain, usecase และ adapter ของตัวเอง |
| Event | ข้อมูลที่บอกว่ามีสิ่งใดเกิดขึ้นแล้ว เช่น `approval.request_approved` |
| การ์ด | ข้อความแชทชนิดพิเศษที่มีข้อมูลและปุ่มของโมดูลอื่น เช่น การ์ดใบลา |
| Ticket | เรื่องแจ้งปัญหาหนึ่งเรื่องใน helpdesk |
| SLA | เวลาที่กำหนดให้ตอบกลับและแก้ไข ticket |

### 1.6 ระดับความสำคัญและรหัส

ความต้องการแต่ละข้อมีรหัส `โมดูล-เลข` ระดับ **Must** (ต้องมีจึงปล่อยได้), **Should** (ควรมี เลื่อนได้ถ้าเวลาไม่พอ) หรือ **Could** (ทำเมื่อมีเวลา) และ milestone ที่ส่งมอบ (M0–M6 ในเฟส 1, P2–P4 คือเฟสถัดไป)

## 2. ผู้ใช้และบทบาท

บทบาทกำหนดเป็นราย workspace ผู้ใช้หนึ่งคนมีบทบาทต่างกันในแต่ละ workspace ได้ ส่วน Agent เป็นบทบาทรายคิวของ helpdesk

| บทบาท | ทำอะไรได้ |
| --- | --- |
| Owner | ทุกอย่างของ Admin และโอนหรือลบ workspace |
| Admin | จัดการสมาชิก บทบาท ช่อง คิว helpdesk ประเภทคำขออนุมัติ และดู audit log |
| Manager | อนุมัติคำขอของสมาชิกในกลุ่มที่ตนเป็นหัวหน้า และดูงานของกลุ่ม |
| Member | ใช้แชท สร้างงาน ส่งคำขออนุมัติ และเปิด ticket |
| Guest | เข้าได้เฉพาะช่องที่ถูกเชิญ สร้างคำขออนุมัติและ ticket ไม่ได้ |
| Agent (รายคิว) | รับและแก้ ticket ในคิวของตน เห็นบันทึกภายใน |

## 3. สถาปัตยกรรม

Server เป็น modular monolith ตัวเดียว แต่ละโมดูลใช้ Clean Architecture ภายใน (domain, usecase, adapter) และคุยกันผ่านสัญญากลางเท่านั้น

<!-- embed: {"id": ".22679", "h": "f8ded9e7", "caption": "สถาปัตยกรรมระบบ · 3 client, 7 โมดูล, 4 บริการพื้นฐาน", "ref": "node/373a3587-6df8"} -->

โมดูลไม่เรียกกันตรง: โมดูลต้นทางส่ง event แล้วโมดูลอื่นมาฟัง จึงปิดโมดูลเส้นประตัวใดก็ได้โดยส่วนที่เหลือยังทำงาน

กฎการพึ่งพา:

- `cmd` ประกอบโมดูล โมดูลพึ่งพา `internal/app` และ `internal/platform` ส่วน `app` ไม่พึ่งพาใคร
- โมดูล import จากโมดูลอื่นได้เฉพาะ `api.go` และ `events.go`
- ภายในโมดูล: adapter เรียก usecase และ usecase เรียก domain โดย domain ไม่รู้จักฐานข้อมูลหรือ HTTP
- `platform` ไม่รู้จักฟีเจอร์ใดเลย

## 4. ระบบโมดูล (PLT)

ปิดโมดูลที่เลือกเปิดได้ตัวใดก็ตาม ระบบส่วนที่เหลือต้องทำงานได้ปกติ และถ้าตั้งค่าผิด ระบบต้องหยุดตั้งแต่ตอนเริ่มพร้อมข้อความที่ชัดเจน

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| PLT-01 | เปิดหรือปิดโมดูลด้วยตัวแปร `MODULES_ENABLED` โดยไม่ต้องแก้โค้ด | Must | M0 |
| PLT-02 | โมดูลประกาศโมดูลที่ขาดไม่ได้ผ่าน `Requires()` ถ้าไม่ครบ server ไม่เริ่มทำงานและแจ้งชื่อโมดูลที่ขาด | Must | M0 |
| PLT-03 | ตรวจการพึ่งพาเป็นวง และเรียงลำดับการเริ่มโมดูลอัตโนมัติ | Must | M0 |
| PLT-04 | Event ที่ไม่มีผู้ฟังไม่ถือเป็น error และ handler ที่ error หรือ panic ต้องไม่กระทบผู้ส่ง | Must | M0 |
| PLT-05 | Service registry: โมดูลประกาศ service ให้โมดูลอื่นค้นหา ถ้าไม่พบให้ใช้ตัวสำรองที่ไม่ทำอะไร | Must | M0 |
| PLT-06 | `GET /api/v1/meta` คืนรายชื่อโมดูลและฟีเจอร์ที่เปิดอยู่ ให้ client ซ่อนเมนู | Must | M0 |
| PLT-07 | Migration แยกตามโมดูล ชื่อตารางนำหน้าด้วยชื่อโมดูล ปิดโมดูลแล้วข้อมูลยังอยู่ | Must | M0 |
| PLT-08 | CI ตรวจขอบเขตโมดูล: ห้าม import ส่วนภายในของโมดูลอื่น | Must | M0 |
| PLT-09 | คิวงานใส่งานได้ใน transaction เดียวกับข้อมูล งานที่ไม่มี worker ถูกยกเลิก ไม่ค้างในคิว | Must | M3 |
| PLT-10 | Event ส่งหลัง commit สำเร็จเท่านั้น ผ่านตาราง outbox และส่งซ้ำได้เมื่อ server ล่ม | Should | M4 |
| PLT-11 | Card registry: โมดูลลงทะเบียนชนิดการ์ด การ์ดของโมดูลที่ปิดแสดงเป็น `fallback_text` | Must | M2 |
| PLT-12 | CI ทดสอบการเริ่มระบบด้วยชุดโมดูลหลายแบบ | Should | M4 |
| PLT-13 | แต่ละ workspace เปิดหรือปิดโมดูลของตัวเองได้ระหว่างใช้งาน | Could | P4 |

## 5. บัญชีและการล็อกอิน (AUTH)

การล็อกอินแยกตามอุปกรณ์ตั้งแต่เฟส 1 เพื่อให้เดสก์ท็อปและมือถือใช้ API เดิมได้

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| AUTH-01 | สมัครด้วยอีเมลและรหัสผ่าน เก็บรหัสผ่านด้วย argon2id | Must | M1 |
| AUTH-02 | ล็อกอินแล้วได้ access token (JWT อายุ 15 นาที) และ refresh token (อายุ 30 วัน) ผูกกับอุปกรณ์ | Must | M1 |
| AUTH-03 | Refresh token เปลี่ยนใหม่ทุกครั้งที่ใช้ ถ้าพบการใช้ token เก่าซ้ำ ให้ยกเลิกทุก token ของอุปกรณ์นั้น | Must | M1 |
| AUTH-04 | ออกจากระบบอุปกรณ์เดียว หรือทุกอุปกรณ์ | Must | M1 |
| AUTH-05 | ดูรายการอุปกรณ์ที่ล็อกอินอยู่ และยกเลิกรายอุปกรณ์ | Should | M1 |
| AUTH-06 | จำกัดการล็อกอินผิด 5 ครั้งต่อนาที ต่ออีเมลและ IP | Must | M1 |
| AUTH-07 | แก้ไขโปรไฟล์: ชื่อที่แสดง รูป ภาษา และเขตเวลา | Must | M1 |
| AUTH-08 | WebSocket ยืนยันตัวตนด้วย access token และตัดการเชื่อมต่อเมื่อ token ถูกยกเลิก | Must | M2 |
| AUTH-09 | ยืนยันอีเมล และรีเซ็ตรหัสผ่านผ่านอีเมล (ลิงก์อายุ 1 ชั่วโมง) | Should | M6 |
| AUTH-10 | ล็อกอินด้วย Google หรือ OIDC | Could | P4 |
| AUTH-11 | ยืนยันตัวตนสองชั้นแบบ TOTP | Could | P4 |

## 6. Workspace และสิทธิ์ (WSP)

ข้อมูลทุกแถวผูกกับ workspace และทุกโมดูลตรวจสิทธิ์ผ่านฟังก์ชันกลางตัวเดียว

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| WSP-01 | สร้าง workspace ผู้สร้างเป็น Owner | Must | M1 |
| WSP-02 | เชิญสมาชิกด้วยอีเมลหรือลิงก์ ลิงก์หมดอายุใน 7 วันและยกเลิกได้ | Must | M1 |
| WSP-03 | บทบาท Owner, Admin, Manager, Member, Guest ตามหัวข้อ 2 | Must | M1 |
| WSP-04 | เปลี่ยนบทบาทและนำสมาชิกออก โดย workspace ต้องมี Owner อย่างน้อยหนึ่งคนเสมอ | Must | M1 |
| WSP-05 | ผู้ใช้หนึ่งคนอยู่ได้หลาย workspace และสลับได้โดยไม่ต้องล็อกอินใหม่ | Must | M1 |
| WSP-06 | ทุกคำสั่งฐานข้อมูลกรองด้วย `workspace_id` ผู้ใช้เข้าถึงข้อมูลข้าม workspace ไม่ได้ | Must | M1 |
| WSP-07 | ตรวจสิทธิ์ผ่านฟังก์ชันกลาง `Can(ผู้ใช้, การกระทำ, ทรัพยากร)` ที่ทุกโมดูลใช้ | Must | M1 |
| WSP-08 | ปิดการใช้งานสมาชิกแล้ว token และการเชื่อมต่อของคนนั้นใน workspace ถูกตัดทันที | Must | M1 |
| WSP-09 | กลุ่ม (แผนกหรือทีม) พร้อมหัวหน้ากลุ่ม ใช้เป็นผู้อนุมัติเริ่มต้น | Should | M4 |
| WSP-10 | Audit log ของการกระทำระดับผู้ดูแล: เชิญ เปลี่ยนบทบาท นำออก เปลี่ยนการตั้งค่า | Should | M6 |

## 7. แชท (CHAT)

ข้อความต้องไม่หาย ไม่ซ้ำ และเรียงลำดับตรงกันทุกอุปกรณ์ โดยใช้เลขลำดับจาก server และ ID ที่ client สร้าง

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| CHAT-01 | ช่องสาธารณะ (ทุกคนใน workspace เข้าได้) และช่องส่วนตัว (เฉพาะผู้ถูกเชิญ) | Must | M2 |
| CHAT-02 | สร้าง แก้ไข เก็บถาวรช่อง และเข้าร่วม ออก เชิญสมาชิก | Must | M2 |
| CHAT-03 | ข้อความส่วนตัวแบบ 1:1 และแบบกลุ่ม | Must | M2 |
| CHAT-04 | ส่งข้อความตัวอักษรรองรับ markdown พื้นฐาน ยาวไม่เกิน 4,000 ตัวอักษร | Must | M2 |
| CHAT-05 | Client แนบ `client_msg_id` ทุกข้อความ ส่งซ้ำด้วย ID เดิมต้องไม่เกิดข้อความซ้ำ | Must | M2 |
| CHAT-06 | Server กำหนดเลขลำดับ `seq` ต่อช่อง เพิ่มทีละหนึ่ง ไม่ซ้ำ ใช้เรียงข้อความ | Must | M2 |
| CHAT-07 | ส่งข้อความใหม่ถึงสมาชิกที่ออนไลน์ผ่าน WebSocket | Must | M2 |
| CHAT-08 | หลังเชื่อมต่อใหม่ client ขอข้อความที่มี `seq` มากกว่าค่าล่าสุดที่มีได้ | Must | M2 |
| CHAT-09 | ดึงประวัติย้อนหลังแบบ cursor ครั้งละไม่เกิน 100 ข้อความ | Must | M2 |
| CHAT-10 | แก้ไขและลบข้อความของตนเอง (ลบแบบ soft delete) Admin ลบข้อความของผู้อื่นได้ | Must | M2 |
| CHAT-11 | ตอบเป็นเธรดใต้ข้อความ | Must | M2 |
| CHAT-12 | สถานะอ่านแล้วต่อช่อง และจำนวนข้อความที่ยังไม่อ่าน | Must | M2 |
| CHAT-13 | @mention ผู้ใช้ และ @channel | Must | M2 |
| CHAT-14 | แนบไฟล์ผ่าน presigned URL ไฟล์ละไม่เกิน 50 MB แสดงตัวอย่างรูปภาพ | Must | M2 |
| CHAT-15 | ข้อความชนิดการ์ด: มี `kind`, `payload` และ `fallback_text` อัปเดตสดเมื่อโมดูลเจ้าของเปลี่ยนสถานะ | Must | M2 |
| CHAT-16 | โมดูลลงทะเบียนเมนูบนข้อความ (เช่น "สร้างงาน") และคำสั่ง slash (เช่น `/ticket`) | Must | M4 |
| CHAT-17 | สถานะออนไลน์ และตัวบอกว่ากำลังพิมพ์ | Should | M2 |
| CHAT-18 | Reaction ด้วยอีโมจิ | Should | M2 |
| CHAT-19 | ค้นหาข้อความด้วย full-text search ของ PostgreSQL เฉพาะช่องที่ผู้ใช้เข้าถึงได้ | Should | P4 |
| CHAT-20 | ปักหมุดข้อความ | Could | P4 |

## 8. การแจ้งเตือน (NOTI)

โมดูลต้นทางกำหนดว่าแจ้งใครด้วยข้อความอะไร ส่วน notify กำหนดว่าจะส่งทางไหนและเมื่อไร ตารางในฐานข้อมูลคือแหล่งข้อมูลจริง

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| NOTI-01 | โมดูลอื่นลงทะเบียนกฎ "event ใด แจ้งใคร ข้อความอะไร" กับ notify โดยไม่เรียก notify ตรง | Must | M3 |
| NOTI-02 | บันทึกแจ้งเตือนทุกรายการลงฐานข้อมูลก่อนส่ง | Must | M3 |
| NOTI-03 | ผู้ใช้ออนไลน์ได้รับแจ้งเตือนผ่าน WebSocket | Must | M3 |
| NOTI-04 | API ดึงรายการ จำนวนที่ยังไม่อ่าน ทำเครื่องหมายอ่านรายตัว และอ่านทั้งหมด | Must | M3 |
| NOTI-05 | ปิด notify แล้วโมดูลอื่นทำงานได้ปกติ (เช่น อนุมัติสำเร็จแต่ไม่มีแจ้งเตือน) | Must | M3 |
| NOTI-06 | แจ้งเตือนข้อความแชท: DM, @mention และตอบในเธรดที่ติดตาม | Must | M3 |
| NOTI-07 | การตั้งค่ารายผู้ใช้: เปิดปิดตามชนิด ปิดเสียงรายช่อง และช่วงเวลาห้ามรบกวน | Should | M6 |
| NOTI-08 | ช่องทางภายนอกส่งผ่านคิวงาน ส่งซ้ำสูงสุด 5 ครั้งแบบหน่วงเวลาเพิ่มขึ้น | Must | P3 |
| NOTI-09 | Push บนมือถือผ่าน Expo Push และลบ token ที่ผู้ให้บริการแจ้งว่าใช้ไม่ได้ | Must | P3 |
| NOTI-10 | ข้อความ push ไม่มีเนื้อหาลับ แอปดึงรายละเอียดเองเมื่อเปิด | Must | P3 |
| NOTI-11 | Web Push บนเบราว์เซอร์ | Should | P4 |
| NOTI-12 | รวมแจ้งเตือนจากช่องเดียวกันที่เกิดถี่ ๆ เป็นรายการเดียว | Should | P4 |
| NOTI-13 | อีเมลสรุปเมื่อไม่อ่านภายใน 1 ชั่วโมง | Could | P4 |

## 9. งาน (TASK)

งานเกิดจากบทสนทนาและแสดงเป็นการ์ดในช่องเดิม เพื่อให้บริบทอยู่กับงาน

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| TASK-01 | สร้างงานจากเมนูบนข้อความ หรือคำสั่ง `/task` | Must | M4 |
| TASK-02 | งานมีชื่อ รายละเอียด ผู้รับผิดชอบ กำหนดส่ง ความสำคัญ และสถานะ (todo, doing, done) | Must | M4 |
| TASK-03 | การ์ดงานในแชทแสดงสถานะล่าสุด และเปลี่ยนสถานะได้จากการ์ด | Must | M4 |
| TASK-04 | หน้า "งานของฉัน" กรองตามสถานะ กำหนดส่ง และช่อง | Must | M4 |
| TASK-05 | ผู้สร้าง ผู้รับผิดชอบ และ Admin แก้ไขงานได้ สมาชิกช่องดูได้ | Must | M4 |
| TASK-06 | แจ้งเตือนเมื่อถูกมอบหมาย เมื่อเหลือ 24 ชั่วโมงก่อนกำหนด และเมื่อเกินกำหนด | Must | M4 |
| TASK-07 | ความเห็นของงานคือเธรดใต้การ์ดงาน | Should | M4 |
| TASK-08 | ส่ง event `task.created`, `task.assigned`, `task.status_changed`, `task.due_soon`, `task.overdue` | Must | M4 |
| TASK-09 | มุมมองบอร์ดของงานในช่อง | Could | P4 |

## 10. คำขออนุมัติ (APR)

ผู้อนุมัติตัดสินได้จากการ์ดในแชท และทุกการตัดสินมีประวัติตรวจสอบย้อนหลังได้

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| APR-01 | ประเภทคำขอเริ่มต้น 2 แบบ: ใบลา (ประเภท วันที่ เหตุผล) และคำขอทั่วไป (หัวข้อ รายละเอียด จำนวนเงิน) | Must | M4 |
| APR-02 | ส่งคำขอโดยเลือกผู้อนุมัติ ค่าเริ่มต้นคือหัวหน้ากลุ่มของผู้ขอ | Must | M4 |
| APR-03 | ผู้อนุมัติได้รับการ์ดคำขอในข้อความส่วนตัว พร้อมปุ่มอนุมัติและปฏิเสธ | Must | M4 |
| APR-04 | เฉพาะผู้อนุมัติที่ระบุตัดสินได้ ตัดสินซ้ำไม่ได้ และการปฏิเสธต้องมีเหตุผล | Must | M4 |
| APR-05 | ผู้ขอยกเลิกคำขอได้ขณะสถานะรออนุมัติ | Must | M4 |
| APR-06 | หน้ารายการ "คำขอของฉัน" และ "รอฉันอนุมัติ" | Must | M4 |
| APR-07 | เก็บประวัติ: ใครทำอะไร เมื่อไร และเหตุผล | Must | M4 |
| APR-08 | แจ้งเตือนผู้อนุมัติเมื่อมีคำขอ และแจ้งผู้ขอเมื่อมีผลตัดสิน | Must | M4 |
| APR-09 | แนบไฟล์ประกอบคำขอ | Should | M4 |
| APR-10 | เตือนผู้อนุมัติซ้ำเมื่อคำขอค้างเกิน 48 ชั่วโมง | Should | P4 |
| APR-11 | อนุมัติหลายขั้นตามลำดับ | Should | P4 |
| APR-12 | Admin สร้างประเภทคำขอและฟิลด์ของฟอร์มเองได้ | Could | P4 |
| APR-13 | ส่ง event `approval.request_created`, `approval.request_approved`, `approval.request_rejected`, `approval.request_cancelled` | Must | M4 |

## 11. Helpdesk (HD)

พนักงานแจ้งปัญหาจากในแชท ทีมที่รับผิดชอบรับเรื่องตามคิว และทุกการเปลี่ยนสถานะเป็นไปตามแผนภาพนี้เท่านั้น

<!-- embed: {"id": ".23202", "h": "e868690e", "caption": "สถานะของ ticket · 6 สถานะ, 1 เส้นทางเปิดใหม่", "ref": "node/e4ec6b5f-839f"} -->

การเปลี่ยนสถานะนอกเส้นทางในแผนภาพต้องถูกปฏิเสธด้วยรหัส 409

| รหัส | ความต้องการ | ระดับ | ส่งมอบ |
| --- | --- | --- | --- |
| HD-01 | Admin สร้างคิว (เช่น IT, อาคาร, HR) และกำหนด Agent ของคิว | Must | M5 |
| HD-02 | หมวดปัญหาผูกกับคิว เลือกหมวดแล้ว ticket เข้าคิวนั้นอัตโนมัติ | Must | M5 |
| HD-03 | สร้าง ticket ได้ 3 ทาง: ฟอร์ม คำสั่ง `/ticket` และเมนูบนข้อความ (ดึงข้อความและไฟล์แนบมาเป็นรายละเอียด) | Must | M5 |
| HD-04 | เลข ticket รันต่อคิว เช่น `IT-0142` | Must | M5 |
| HD-05 | สถานะและการเปลี่ยนสถานะตามแผนภาพ ตรวจในชั้น domain | Must | M5 |
| HD-06 | Agent กดรับงานเอง หรือ Admin มอบหมายให้ Agent ในคิว | Must | M5 |
| HD-07 | ความเห็น 2 แบบ: สาธารณะ (ผู้แจ้งเห็น) และภายใน (เฉพาะ Agent และ Admin) | Must | M5 |
| HD-08 | การ์ด ticket ในแชทแสดงเลข สถานะ ผู้รับผิดชอบ มีปุ่มรับงาน และอัปเดตสด | Must | M5 |
| HD-09 | ความสำคัญ 4 ระดับ: ต่ำ ปกติ สูง เร่งด่วน | Must | M5 |
| HD-10 | หน้ารายการ ticket กรองตามคิว สถานะ ผู้รับผิดชอบ และ "ที่ฉันแจ้ง" | Must | M5 |
| HD-11 | นโยบาย SLA ตามความสำคัญ: เวลาตอบครั้งแรกและเวลาแก้ไข คิดเฉพาะเวลาทำการ และหยุดนับในสถานะ Waiting | Should | P4 |
| HD-12 | เตือนผู้รับผิดชอบเมื่อใช้เวลาไป 80% และแจ้งหัวหน้าคิวเมื่อเกินกำหนด | Should | P4 |
| HD-13 | ปิด ticket อัตโนมัติเมื่ออยู่ในสถานะ Resolved ครบ 3 วัน | Should | P4 |
| HD-14 | ช่องรับเรื่องอัตโนมัติ: ข้อความใหม่ในช่องที่กำหนดกลายเป็น ticket | Should | P4 |
| HD-15 | กระจายงานอัตโนมัติแบบวนรอบ หรือให้คนที่มีงานค้างน้อยที่สุด | Should | P4 |
| HD-16 | ผู้แจ้งให้คะแนนความพึงพอใจเมื่อปิด ticket | Could | P4 |
| HD-17 | รายงาน: จำนวน ticket เวลาแก้ไขเฉลี่ย และสัดส่วนที่ทัน SLA ต่อคิว | Should | P4 |
| HD-18 | บันทึกประวัติทุกการเปลี่ยนแปลงของ ticket | Must | M5 |
| HD-19 | ถ้าเปิดโมดูล task สร้างงานต่อจาก ticket ได้ ถ้าปิดอยู่ปุ่มนี้ถูกซ่อนและ helpdesk ทำงานปกติ | Should | M5 |
| HD-20 | ส่ง event ตามรายการในหัวข้อ 14.4 และลงทะเบียนกฎแจ้งเตือน | Must | M5 |

## 12. ความต้องการที่ไม่ใช่ฟังก์ชัน (NFR)

ตัวเลขด้านประสิทธิภาพเป็นเป้าหมายเบื้องต้น วัดบนเครื่อง 2 vCPU และหน่วยความจำ 4 GB ปรับได้หลังทดสอบโหลดครั้งแรกใน M6

| รหัส | หมวด | ความต้องการ | เกณฑ์วัด |
| --- | --- | --- | --- |
| NFR-01 | ประสิทธิภาพ | เวลาตอบของ API ทั่วไป | p95 ต่ำกว่า 200 ms |
| NFR-02 | ประสิทธิภาพ | เวลาจากส่งข้อความถึงผู้รับที่ออนไลน์ | p95 ต่ำกว่า 300 ms |
| NFR-03 | ประสิทธิภาพ | การเชื่อมต่อ WebSocket พร้อมกันต่อเครื่อง | 1,000 การเชื่อมต่อ |
| NFR-04 | การขยาย | Server ไม่เก็บสถานะในหน่วยความจำ รันหลายเครื่องได้โดยใช้ Redis กระจายข้อความ | ทดสอบ 2 เครื่องแล้วข้อความถึงครบ |
| NFR-05 | ความปลอดภัย | บังคับ TLS ในการใช้งานจริง รหัสผ่านเก็บด้วย argon2id ความลับอ่านจาก environment | ตรวจใน code review |
| NFR-06 | ความปลอดภัย | แยกข้อมูลราย workspace | มีเทสต์ยืนยันว่าเข้าข้าม workspace ไม่ได้ ทุกโมดูล |
| NFR-07 | ความปลอดภัย | ตรวจข้อมูลนำเข้าทุก endpoint จำกัดอัตราการเรียก และ presigned URL อายุ 15 นาที | เทสต์ระดับ API |
| NFR-08 | ความปลอดภัย | ผู้ใช้ส่งออกและขอลบข้อมูลส่วนบุคคลของตนได้ (รองรับ PDPA) | Should, P4 |
| NFR-09 | ความเสถียร | ข้อความแชทไม่หายและไม่ซ้ำเมื่อเครือข่ายหลุดหรือส่งซ้ำ | เทสต์จำลองการส่งซ้ำและเชื่อมต่อใหม่ |
| NFR-10 | ความเสถียร | Event ส่งอย่างน้อยหนึ่งครั้ง และ handler รับซ้ำได้โดยผลไม่เปลี่ยน | เทสต์ส่ง event ซ้ำ |
| NFR-11 | ความเสถียร | ปิด server แบบรอให้ request และงานที่ค้างเสร็จ มี health และ readiness check | ทดสอบตอน deploy |
| NFR-12 | การดูแลรักษา | เทสต์ครอบคลุมชั้น domain และ usecase | ไม่ต่ำกว่า 80% |
| NFR-13 | การดูแลรักษา | OpenAPI สร้างจากโค้ดอัตโนมัติ และ type ฝั่งเว็บสร้างจาก OpenAPI | CI ล้มเมื่อไฟล์ที่สร้างไม่ตรงกับโค้ด |
| NFR-14 | การติดตาม | Log แบบ JSON มี request ID และ metrics แบบ Prometheus | มี endpoint `/metrics` |
| NFR-15 | การติดตั้ง | ติดตั้งครบด้วย `docker compose up` คำสั่งเดียว รองรับ amd64 และ arm64 | ทดสอบบนเครื่องเปล่า |
| NFR-16 | การใช้งาน | เว็บรองรับ Chrome, Firefox, Safari และ Edge สองเวอร์ชันล่าสุด จอกว้างตั้งแต่ 360 px | ทดสอบด้วยมือก่อนปล่อย |
| NFR-17 | ภาษาและเวลา | หน้าจอภาษาไทยและอังกฤษ เก็บเวลาเป็น UTC แสดงตามเขตเวลาของผู้ใช้ | เทสต์ระดับหน้าจอ |
| NFR-18 | Open source | มี README, LICENSE, CONTRIBUTING และเอกสารสถาปัตยกรรม ใช้เลขเวอร์ชันแบบ semantic versioning | ตรวจก่อนปล่อย v0.1 |

## 13. โมเดลข้อมูล

ทุกตารางใช้ ID แบบ UUIDv7 เก็บเวลาเป็น `timestamptz` (UTC) และตารางที่เป็นข้อมูลขององค์กรมีคอลัมน์ `workspace_id` โมดูลหนึ่งห้ามอ่านหรือเขียนตารางของโมดูลอื่น

| โมดูล | ตาราง | คอลัมน์สำคัญ |
| --- | --- | --- |
| auth | `auth_users` | email (ไม่ซ้ำ), password_hash, display_name, avatar_url, locale, timezone |
| auth | `auth_devices` | user_id, name, platform, push_token, last_seen_at |
| auth | `auth_refresh_tokens` | device_id, token_hash, expires_at, revoked_at |
| workspace | `workspace_workspaces` | name, slug (ไม่ซ้ำ) |
| workspace | `workspace_members` | workspace_id, user_id, role, status |
| workspace | `workspace_invites` | workspace_id, email, token_hash, role, expires_at |
| workspace | `workspace_groups`, `workspace_group_members` | name; group_id, user_id, is_manager |
| workspace | `workspace_audit_logs` | actor_id, action, target, data |
| chat | `chat_channels` | workspace_id, type (public, private, dm), name, last_seq, archived_at |
| chat | `chat_channel_members` | channel_id, user_id, last_read_seq, muted |
| chat | `chat_messages` | channel_id, seq, sender_id, client_msg_id, kind, body, payload (jsonb), fallback_text, thread_root_id, edited_at, deleted_at |
| chat | `chat_attachments` | message_id, object_key, file_name, size, mime_type |
| chat | `chat_reactions` | message_id, user_id, emoji |
| notify | `notify_notifications` | user_id, kind, title, body, link, data, read_at |
| notify | `notify_preferences` | user_id, scope (ชนิดหรือช่อง), setting |
| task | `task_items` | channel_id, message_id, title, description, creator_id, assignee_id, due_at, priority, status |
| approval | `approval_requests` | type, requester_id, status, payload (jsonb), message_id |
| approval | `approval_steps` | request_id, step_no, approver_id, status, reason, decided_at |
| helpdesk | `helpdesk_queues`, `helpdesk_queue_members` | key (เช่น IT), name, assign_strategy; queue_id, user_id |
| helpdesk | `helpdesk_categories` | queue_id, name |
| helpdesk | `helpdesk_tickets` | queue_id, number, title, description, requester_id, assignee_id, status, priority, category_id, resolve_due_at, resolved_at, closed_at |
| helpdesk | `helpdesk_comments` | ticket_id, author_id, body, is_internal |
| helpdesk | `helpdesk_activities` | ticket_id, actor_id, action, from_value, to_value |
| platform | `outbox_events` | name, payload, created_at, published_at |

ข้อบังคับสำคัญ: `chat_messages` ไม่ซ้ำที่ (channel_id, seq) และ (sender_id, client_msg_id) ส่วน `helpdesk_tickets` ไม่ซ้ำที่ (queue_id, number)

## 14. ส่วนต่อประสาน

### 14.1 มาตรฐาน REST API

- เส้นทางขึ้นต้นด้วย `/api/v1/{โมดูล}/...` รับและส่ง JSON
- ยืนยันตัวตนด้วย header `Authorization: Bearer <access token>` และระบุ workspace ด้วย header `X-Workspace-ID`
- รายการใช้ cursor: `?cursor=&limit=` (สูงสุด 100) ตอบกลับ `{ "items": [...], "next_cursor": "..." }`
- Error ใช้รูปแบบเดียว: `{ "error": { "code": "forbidden", "message": "...", "details": {} } }`
- รหัสสถานะ: 400 ข้อมูลไม่ถูกต้อง, 401 ไม่ได้ล็อกอิน, 403 ไม่มีสิทธิ์, 404 ไม่พบ, 409 สถานะขัดแย้ง, 429 เรียกถี่เกินไป
- Endpoint ของโมดูลที่ปิดอยู่ตอบ 404

### 14.2 Endpoint หลัก

| โมดูล | Endpoint |
| --- | --- |
| auth | `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/me`, `GET /auth/devices` |
| workspace | `POST /workspaces`, `GET /workspaces`, `POST /workspaces/{id}/invites`, `GET /workspaces/{id}/members`, `PATCH /workspaces/{id}/members/{userId}` |
| chat | `GET /chat/channels`, `POST /chat/channels`, `GET /chat/channels/{id}/messages?after_seq=`, `POST /chat/channels/{id}/messages`, `PATCH /chat/messages/{id}`, `DELETE /chat/messages/{id}`, `POST /chat/channels/{id}/read`, `POST /chat/uploads` |
| notify | `GET /notify/notifications`, `GET /notify/unread-count`, `POST /notify/notifications/{id}/read`, `POST /notify/read-all` |
| task | `POST /task/items`, `GET /task/items?assignee=me&status=`, `PATCH /task/items/{id}` |
| approval | `POST /approval/requests`, `GET /approval/requests?role=requester`, `POST /approval/requests/{id}/approve`, `POST /approval/requests/{id}/reject`, `POST /approval/requests/{id}/cancel` |
| helpdesk | `POST /helpdesk/tickets`, `GET /helpdesk/tickets?queue=&status=`, `POST /helpdesk/tickets/{id}/assign`, `POST /helpdesk/tickets/{id}/status`, `POST /helpdesk/tickets/{id}/comments`, `GET /helpdesk/queues` |
| ระบบ | `GET /meta`, `GET /healthz`, `GET /readyz`, `GET /metrics` |

### 14.3 โปรโตคอล WebSocket

- เชื่อมต่อที่ `GET /api/v1/ws` ส่ง access token ในข้อความแรก server ตอบ `ready` แล้วจึงเริ่มส่งข้อมูล
- ทุกข้อความใช้ซอง `{ "type": "...", "data": { ... } }`
- Server ส่งข้อมูลของทุกช่องที่ผู้ใช้เป็นสมาชิกโดยอัตโนมัติ client ไม่ต้อง subscribe รายช่อง
- Client ส่ง `ping` ทุก 30 วินาที ถ้า server ไม่ได้รับภายใน 60 วินาทีให้ตัดการเชื่อมต่อ
- หลังเชื่อมต่อใหม่ client เรียก REST `messages?after_seq=` ของแต่ละช่องเพื่อเติมข้อความที่พลาด WebSocket ไม่ส่งย้อนหลัง

| ทิศทาง | ชนิดข้อความ |
| --- | --- |
| server → client | `ready`, `message.created`, `message.updated`, `message.deleted`, `card.updated`, `channel.read`, `channel.updated`, `presence.changed`, `typing`, `notification.created` |
| client → server | `auth`, `ping`, `typing` |

### 14.4 รายการ event ระหว่างโมดูล

| Event | ผู้ส่ง | ผู้ฟัง |
| --- | --- | --- |
| `workspace.member_removed` | workspace | auth (ตัด token), chat (นำออกจากช่อง) |
| `chat.message_created` | chat | notify (DM, mention), helpdesk (ช่องรับเรื่องอัตโนมัติ) |
| `task.assigned`, `task.due_soon`, `task.overdue` | task | notify |
| `task.status_changed` | task | chat (อัปเดตการ์ด) |
| `approval.request_created` | approval | chat (ส่งการ์ด), notify |
| `approval.request_approved`, `approval.request_rejected` | approval | chat (อัปเดตการ์ด), notify |
| `helpdesk.ticket_created`, `helpdesk.ticket_assigned` | helpdesk | chat (การ์ด), notify |
| `helpdesk.comment_added`, `helpdesk.ticket_resolved` | helpdesk | chat, notify |
| `helpdesk.sla_warning`, `helpdesk.sla_breached` | helpdesk | notify |

ชื่อ event ใช้รูป `โมดูล.สิ่งที่เกิดขึ้น` เป็นอดีตกาล เพิ่มฟิลด์ได้ แต่ห้ามลบหรือเปลี่ยนความหมายของฟิลด์เดิม

## 15. แผนพัฒนา

เฟส 1 ใช้ 12 สัปดาห์ถึง v0.1 โดยสมมติว่ามีนักพัฒนาหนึ่งคนทำเต็มเวลา ถ้าทำนอกเวลางานให้คูณระยะเวลาตามชั่วโมงที่มีจริง แต่ละ milestone ส่งมอบทั้ง server และหน้าเว็บของส่วนนั้น

| Milestone | สัปดาห์ | สิ่งที่ส่งมอบ | ความต้องการ |
| --- | --- | --- | --- |
| M0 โครงโปรเจกต์ | 1 | Monorepo, Docker Compose, CI, `internal/app`, `internal/platform`, ระบบลงทะเบียนโมดูล, โครงเว็บและ `packages/core` | PLT-01 ถึง 08 |
| M1 บัญชีและ workspace | 2–3 | สมัคร ล็อกอิน token รายอุปกรณ์ สร้าง workspace เชิญสมาชิก บทบาทและสิทธิ์ | AUTH-01 ถึง 07, WSP-01 ถึง 08 |
| M2 แชทเรียลไทม์ | 4–6 | ช่อง DM ข้อความ เธรด ไฟล์แนบ อ่านแล้ว การซิงก์หลังเชื่อมต่อใหม่ และกลไกการ์ด | CHAT-01 ถึง 15, 17, 18, AUTH-08, PLT-11 |
| M3 แจ้งเตือน | 7 | โมดูล notify ช่องทาง WebSocket กระดิ่งและรายการแจ้งเตือนบนเว็บ คิวงาน | NOTI-01 ถึง 06, PLT-09 |
| M4 งานและคำขออนุมัติ | 8–9 | โมดูล task และ approval การ์ดในแชท เมนูบนข้อความ คำสั่ง slash กลุ่มและหัวหน้า outbox | TASK-01 ถึง 08, APR-01 ถึง 09, 13, CHAT-16, WSP-09, PLT-10, 12 |
| M5 Helpdesk รุ่นแรก | 10–11 | คิว หมวด ticket สถานะ ความเห็นสาธารณะและภายใน การ์ด การแจ้งเตือน | HD-01 ถึง 10, 18 ถึง 20 |
| M6 ปล่อย v0.1 | 12 | ทดสอบโหลด แก้จุดอ่อน การตั้งค่าแจ้งเตือน ยืนยันอีเมล audit log README เอกสารสถาปัตยกรรม เดโมออนไลน์ | AUTH-09, WSP-10, NOTI-07, NFR ทั้งหมด |
| P2 เดสก์ท็อป | 13–14 | แอป Tauri: แจ้งเตือนของระบบ ไอคอน system tray อัปเดตอัตโนมัติ | — |
| P3 มือถือ | 15–19 | แอป Expo iOS และ Android: push, cache ออฟไลน์, build ผ่าน EAS | NOTI-08 ถึง 10 |
| P4 ต่อยอด | 20 เป็นต้นไป | SLA และรายงานของ helpdesk, Web Push, ค้นหา, อนุมัติหลายขั้น | HD-11 ถึง 17, NOTI-11 ถึง 13, CHAT-19, APR-10 ถึง 12 |

ลำดับนี้มีเหตุผลจากการพึ่งพา: ทุกโมดูลต้องใช้ auth และ workspace การ์ดของ task, approval และ helpdesk ต้องใช้ chat และ notify ควรมีก่อนโมดูลบริหาร เพื่อให้โมดูลเหล่านั้นลงทะเบียนกฎแจ้งเตือนได้ตั้งแต่วันแรก

## 16. การทดสอบและเกณฑ์รับงาน

### 16.1 ระดับการทดสอบ

| ระดับ | ทดสอบอะไร | เครื่องมือ |
| --- | --- | --- |
| Unit | กฎใน domain และลำดับใน usecase โดยใช้ตัวปลอมแทนฐานข้อมูล | `go test`, testify |
| Integration | ชั้น adapter กับ PostgreSQL และ Redis จริง | testcontainers-go |
| API | ยิง HTTP และ WebSocket ใส่ server ทั้งตัว รวมกรณีไม่มีสิทธิ์ | `httptest` |
| ชุดโมดูล | เริ่ม server ด้วยชุดโมดูลหลายแบบ | เทสต์ใน `internal/app` |
| หน้าเว็บ | เส้นทางหลักของผู้ใช้ตั้งแต่ต้นจนจบ | Playwright |
| โหลด | เป้าหมาย NFR-01 ถึง 03 | k6 |

### 16.2 นิยามของคำว่าเสร็จ

งานหนึ่งชิ้นถือว่าเสร็จเมื่อครบทุกข้อ:

- มีเทสต์ครอบคลุมกรณีสำเร็จ กรณีไม่มีสิทธิ์ และกรณีข้อมูลไม่ถูกต้อง
- CI ผ่านทั้ง lint เทสต์ และการตรวจขอบเขตโมดูล
- OpenAPI และ type ฝั่งเว็บสร้างใหม่แล้ว
- มี migration และรันบนฐานข้อมูลเปล่าได้
- อัปเดตเอกสารถ้าพฤติกรรมที่ผู้ใช้เห็นเปลี่ยน

### 16.3 เกณฑ์รับงานราย milestone

| Milestone | รับงานเมื่อ |
| --- | --- |
| M0 | `docker compose up` แล้วเปิดเว็บได้ `GET /meta` แสดงโมดูลที่เปิด และตั้งค่าโมดูลขาด dependency แล้ว server ไม่เริ่มพร้อมข้อความที่ระบุชื่อโมดูล |
| M1 | ผู้ใช้สองคนสมัคร คนหนึ่งสร้าง workspace แล้วเชิญอีกคนเข้าได้ ผู้ใช้นอก workspace เรียกข้อมูลแล้วได้ 403 และ token เก่าที่ถูกหมุนแล้วใช้ซ้ำไม่ได้ |
| M2 | ผู้ใช้สองคนคุยกันในสองเบราว์เซอร์เห็นข้อความภายใน 1 วินาที ตัดเน็ตแล้วต่อใหม่ข้อความครบและเรียงถูก ส่งข้อความเดิมซ้ำไม่เกิดข้อความซ้ำ |
| M3 | ถูก @mention แล้วกระดิ่งขึ้นจำนวนทันที เปิดเว็บใหม่จำนวนยังถูกต้อง และปิดโมดูล notify แล้วแชททำงานปกติ |
| M4 | สร้างงานจากข้อความแล้วเปลี่ยนสถานะจากการ์ดได้ ส่งใบลาแล้วหัวหน้าอนุมัติจากการ์ด การ์ดของผู้ขอเปลี่ยนสถานะทันที และผู้ที่ไม่ใช่ผู้อนุมัติกดแล้วได้ 403 |
| M5 | เปิด ticket จากข้อความ Agent กดรับและเปลี่ยนสถานะได้ครบทุกเส้นทางในแผนภาพสถานะ ผู้แจ้งไม่เห็นบันทึกภายใน และปิดโมดูล task แล้ว helpdesk ยังทำงาน |
| M6 | ผ่านเป้าหมาย NFR-01 ถึง 03 คนนอกทำตาม README แล้วติดตั้งได้ภายใน 10 นาที และเดโมออนไลน์ใช้งานได้ |

## 17. ประเด็นที่ยังต้องตัดสินใจ

ควรสรุปข้อ 1–3 ก่อนเริ่ม M0 เพราะกระทบแผนและชื่อใน repo

- ชื่อโปรเจกต์สรุปแล้วว่า Samnak Chat เหลือตรวจว่าชื่อว่าง (GitHub, npm, โดเมน) และตั้งชื่อ repo
- จำนวนคนและชั่วโมงต่อสัปดาห์ที่ทำได้จริง เพื่อปรับระยะเวลาในหัวข้อ 15
- สัญญาอนุญาต: MIT, Apache-2.0 หรือ AGPL-3.0
- ภาษาเริ่มต้นของหน้าจอ: ไทยหรืออังกฤษ
- ที่ตั้งของเดโมออนไลน์และงบประมาณ
- อนุมัติหลายขั้น (APR-11) ควรเข้า v0.1 หรือไม่
- ผู้ใช้ Guest จำเป็นใน v0.1 หรือเลื่อนไปเฟส 4
- ค่าเริ่มต้นของ SLA และเวลาทำการของ helpdesk
