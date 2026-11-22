# ตั้งค่าสำหรับพัฒนา

หน้านี้ทำให้คุณมีสำเนาสำหรับพัฒนาของ Samnak Chat ทำงานอยู่บนเครื่องของคุณ ทั้งเซิร์ฟเวอร์ API เว็บแอป และบริการที่ทั้งสองต้องใช้ ถ้าต้องการรันชุดติดตั้งสำหรับใช้งานจริงบนเซิร์ฟเวอร์ ให้ดู[ติดตั้งบนเซิร์ฟเวอร์ของคุณ](/th/install)

## สิ่งที่ต้องมี

- Go 1.26 หรือใหม่กว่า
- Node 22 หรือใหม่กว่า
- Docker สำหรับ PostgreSQL, Redis, MinIO และ Mailpit
- Git
- `golangci-lint` เฉพาะเมื่อคุณจะรัน `make lint`

## เริ่มต้น

```sh
git clone https://github.com/KPV2004/samnak-chat.git
cd samnak-chat
cp .env.example .env
npm install
make up          # PostgreSQL, Redis, MinIO และ Mailpit ใน Docker
make migrate     # สร้างตารางของโมดูลที่เปิดอยู่
make server      # API ที่ http://localhost:8080
make web         # เว็บแอปที่ http://localhost:5173
```

เปิด `http://localhost:5173` เว็บแอปส่งต่อ `/api` รวมถึง WebSocket ที่ `/api/v1/ws` ไปยัง API ที่พอร์ต 8080

`MODULES_ENABLED` ใน `.env` เป็นตัวกำหนดว่าโมดูลเสริมตัวไหนทำงาน ไฟล์ `.env.example` เปิด `notify`, `task`, `approval` และ `helpdesk` ไว้ เว็บแอปแสดงเฉพาะเมนูของโมดูลที่เซิร์ฟเวอร์รายงานว่ามี

## อ่านอีเมล

เมื่อตั้ง `SMTP_HOST=localhost` ใน `.env` เซิร์ฟเวอร์จะส่งเมลไปที่ Mailpit เปิดกล่องขาเข้าที่ `http://localhost:8025` ถ้า `SMTP_HOST` ว่าง เซิร์ฟเวอร์จะเขียนอีเมลแต่ละฉบับลงบันทึกของตัวเองแทน

## คำสั่ง

| คำสั่ง | สิ่งที่ทำ |
| --- | --- |
| `make up` | เริ่ม PostgreSQL, Redis, MinIO และ Mailpit ด้วย `deploy/docker-compose.yml` |
| `make down` | หยุดบริการเหล่านั้น ข้อมูลยังอยู่ |
| `make migrate` | รัน migration ของโมดูลที่เปิดอยู่ |
| `make server` | รันเซิร์ฟเวอร์ API ที่พอร์ต 8080 |
| `make web` | รันเว็บแอปที่พอร์ต 5173 |
| `make openapi` | สร้าง `apps/server/api/openapi.yaml` และไทป์ใน `packages/core` ใหม่ |
| `make test` | `go test ./...` ใน `apps/server` แล้วตามด้วย `npm test` |
| `make lint` | `go vet`, `golangci-lint` และ `npm run lint` |
| `npm run build` | ตรวจไทป์และ build เว็บแอปกับแพ็กเกจ |
| `bash scripts/coverage.sh [80]` | แสดงความครอบคลุมของทุกแพ็กเกจ domain และ use-case และล้มเหลวเมื่อต่ำกว่าเกณฑ์ |
| `bash scripts/check-modules.sh <MODULES_ENABLED หรือไม่ตั้ง> <โมดูลเสริม>` | เริ่มเซิร์ฟเวอร์ด้วยชุดโมดูลหนึ่ง แล้วตรวจ `/healthz`, `/meta` และเส้นทางของโมดูลเสริม |

ตัวอย่าง `bash scripts/check-modules.sh task task` จะเริ่มเซิร์ฟเวอร์ด้วย `MODULES_ENABLED=task` และตรวจว่ามีเฉพาะเส้นทางของ task ที่ถูกติดตั้ง

## ทำงานกับเว็บไซต์เอกสารนี้

เว็บไซต์อยู่ใน `apps/docs` สร้างด้วย VitePress

```sh
npm run docs:dev -w apps/docs     # ดูตัวอย่างในเครื่อง พร้อมรีโหลดอัตโนมัติ
npm run docs:build -w apps/docs   # build ไปที่ apps/docs/.vitepress/dist
```

มีสองหน้าที่มาจากไฟล์ในที่เก็บโค้ดและไม่ถูกแก้ด้วยมือ คือหน้าอ้างอิง API ซึ่งสร้างจาก `apps/server/api/openapi.yaml` และหน้าบันทึกการเปลี่ยนแปลงซึ่งสร้างจาก `CHANGELOG.md` การ build จะเขียนสองหน้านี้ก่อน ดังนั้นให้แก้ที่ไฟล์ต้นทางแทน การ build จะล้มเหลวเมื่อมีลิงก์ที่ใช้ไม่ได้ ตรวจลิงก์ก่อน push
