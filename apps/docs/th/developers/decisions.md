# บันทึกการตัดสินใจ

บันทึกการตัดสินใจทางสถาปัตยกรรม (ADR) บอกว่าเลือกอะไร เพราะอะไร และอะไรถูกปฏิเสธ แต่ละการตัดสินใจมีไฟล์ของตัวเองในที่เก็บโค้ด ที่ [`docs/adr/`](https://github.com/KPV2004/samnak-chat/tree/main/docs/adr) ไฟล์เหล่านี้ระบุสถานะ บริบท การตัดสินใจ ผลที่ตามมา และทางเลือกอื่น

| ลำดับ | การตัดสินใจ | เหตุผลโดยย่อ | บันทึก |
| --- | --- | --- | --- |
| 0001 | โมโนลิธแบบแยกโมดูลแทนไมโครเซอร์วิส | ทีมเล็กดูแลง่าย และแยกโมดูลออกได้ในภายหลัง | [0001](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0001-modular-monolith.md) |
| 0002 | Clean Architecture ภายในแต่ละโมดูล | ทดสอบกฎธุรกิจได้โดยไม่ต้องมีฐานข้อมูล | [0002](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0002-clean-architecture-per-module.md) |
| 0003 | อีเวนต์บัสพร้อม outbox | โมดูลแยกจากกัน และอีเวนต์ไม่หายเมื่อระบบล่ม | [0003](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0003-event-bus-with-outbox.md) |
| 0004 | Chi บน `net/http` | ใช้ได้กับทุกไลบรารีมาตรฐานของ Go | [0004](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0004-chi-on-net-http.md) |
| 0005 | sqlc แทน ORM | SQL ชัดเจน และตรวจไทป์ได้ตั้งแต่คอมไพล์ | [0005](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0005-sqlc-instead-of-orm.md) |
| 0006 | River แทนคิวแยกต่างหาก | ใช้ PostgreSQL เดิมซ้ำ และเข้าคิวงานในทรานแซกชันได้ | [0006](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0006-river-job-queue.md) |
| 0007 | `seq` แยกตามช่องและ `client_msg_id` | ไม่มีข้อความซ้ำ และลำดับเดียวกันบนทุกอุปกรณ์ | [0007](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0007-per-channel-seq-and-client-msg-id.md) |
| 0008 | เว็บแยกจากมือถือ ใช้ `packages/core` ร่วมกัน | แต่ละแพลตฟอร์มได้ UI ที่เหมาะ โดยไม่ต้องเขียนตรรกะซ้ำ | [0008](https://github.com/KPV2004/samnak-chat/blob/main/docs/adr/0008-web-and-mobile-separate-shared-core.md) |

## การเขียนบันทึกใหม่

ตั้งชื่อไฟล์ `NNNN-ชื่อเรื่อง.md` โดยใช้เลขถัดไปที่ว่างอยู่ ให้มีหัวข้อเหมือนบันทึกเดิม คือ Status, Context, Decision, Consequences และ Alternatives considered แล้วลิงก์บันทึกใหม่จากตารางใน [`docs/architecture.md`](https://github.com/KPV2004/samnak-chat/blob/main/docs/architecture.md) หัวข้อ 16
