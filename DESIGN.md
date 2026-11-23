# Design Direction: Task & Bug Tracker

Arah visual untuk aplikasi ini. `antislop.md` adalah filter yang dipakai di atas arah ini, bukan pengganti arah.

## Identitas

**Konsol editorial.** Alat kerja internal untuk tim yang mengelola task dan bug. Karakter: tenang, padat, bisa dipercaya. Bukan landing page, bukan dashboard korporat yang templat. Permukaan tenang, data terbaca, satu aksen indigo untuk hal yang butuh tindakan.

Dial: **ENERGY 2 / RHYTHM 2 / MOTION 1**

- ENERGY 2: menyapa dengan tegas lewat tipografi dan hierarki, bukan dekorasi.
- RHYTHM 2: komposisi per layar bervariasi, tapi konsisten di dalam satu produk.
- MOTION 1: transisi pendek untuk orientasi (hover, buka/tutup, muncul), tanpa gerak dekoratif.

## Palet

Token dipakai di `frontend/src/styles.css` (`:root` dan `[data-theme='dark']`).

| Peran | Token | Light | Dark | Catatan |
|---|---|---|---|---|
| Aksen | `--accent` | `#6366F1` | `#8B88F5` | CTA, state aktif, tautan, focus ring, ikon interaktif |
| Isi tombol aksen | `--accent-fill` | `#5652E6` | `#8B88F5` | Satu langkah lebih gelap di mode terang supaya label putih lolos AA (5.61:1) |
| Hover aksen | `--accent-strong` | `#4F46E5` | `#A5A2F7` | |
| Teks di atas isi aksen | `--accent-ink` | `#FFFFFF` | `#14161A` | |
| Teks aksen | `--accent-text` | `#4F46E5` | `#A5A2F7` | 6.29:1 di mode terang |
| Sukses | `--success` | `#10B981` | `#34D399` | Status selesai dan konfirmasi |
| Perhatian | `--warn` | `#F59E0B` | `#FBBF24` | Ikon dan garis status perhatian |
| Teks perhatian | `--warn-text` | `#92400E` | `#FCD34D` | `#F59E0B` polos hanya 2.15:1, jadi teks memakai nada gelap |
| Kritis | `--danger` | `#EF4444` | `#F87171` | |
| Teks kritis | `--danger-text` | `#B91C1C` | `#FCA5A5` | `#EF4444` polos hanya 3.76:1, jadi teks memakai nada gelap |
| Teks utama | `--ink` | `#0A0A0A` | `#F5F5F7` | |
| Teks sekunder | `--ink-2` | `#4A4A55` | `#C3C3CC` | |
| Teks redup | `--ink-3` | `#6B6B7B` | `#9A9AA6` | 5.23:1 di mode terang |
| Latar halaman | `--surface-page` | `#FAFAFA` | `#131317` | |
| Permukaan | `--surface` | `#FFFFFF` | `#1C1C22` | Kartu, panel, modal |
| Permukaan 2 | `--surface-2` | `#F5F5F7` | `#24242C` | Isian halus |
| Permukaan 3 | `--surface-3` | `#EBEBEF` | `#2D2D36` | Hover dan nonaktif |
| Blok tinta | `--surface-ink` | `#0A0A0A` | `#F5F5F7` | Item menu aktif, avatar |
| Garis | `--line` | `#E8E8EC` | `#2E2E36` | Garis rambut 1px |
| Garis kuat | `--line-strong` | `#D6D6DC` | `#3D3D47` | Border input |
| Scrim | `--scrim` | `rgba(10,10,10,.55)` | sama | Overlay drawer dan dialog |

`#20970B` (hijau sekunder) didefinisikan di DESIGN.md asal Genesis untuk highlight homepage. Aplikasi ini login-only dan tidak punya homepage, jadi token itu **tidak dipakai**. Jangan dipakai untuk hal lain.

## Tipografi

- **Display:** General Sans, dipakai untuk heading.
- **Body dan UI:** DM Sans.
- **Kode:** JetBrains Mono, hanya untuk hal yang harus disalin apa adanya (slug workspace, id, password sementara, potongan kode bug).

Ketiga font di-host sendiri di `frontend/src/fonts` (General Sans dari Fontshare, DM Sans dan JetBrains Mono disalin dari paket `@fontsource`). Tidak ada request ke CDN pihak ketiga saat render, dan tidak ada fallback `.woff` yang menumpuk ukuran bundle.

Heading memakai bobot 600 dengan letter-spacing `-0.02em`. Seluruh aplikasi memakai **dua bobot saja**, 400 dan 600, sesuai aturan Genesis "no more than two font weights on a single screen". Skala praktis: 12 / 13 / 15 / 17 / 20 / 26 px. Label kecil memakai sentence case, bukan kapital penuh.

Skala Display 72px dan Headline 60px dari Genesis dipakai untuk halaman marketing. Aplikasi ini login-only dan tidak punya halaman itu, jadi ukuran segitu tidak punya permukaan.

## Elevasi

- Bayangan hanya untuk lapisan yang benar-benar melayang dan untuk interaksi, tidak untuk elemen statis.
- Kartu: rata dengan garis 1px, saat hover dapat bayangan `0 8px 30px rgba(0,0,0,.08)`.
- Tombol primary: saat hover dapat glow indigo `0 4px 12px rgba(99,102,241,.35)`.
- Fokus: ring indigo. Diimplementasikan sebagai outline 2px `var(--accent)` karena ring berbasis box-shadow akan menimpa bayangan hover kartu dan tombol.
- Overlay drawer dan dialog memakai `--scrim`.

Backdrop-blur pada navigasi **tidak dipakai**. Topbar tidak menimpa konten yang bergulir (konten bergulir di container tersendiri), jadi blur tidak punya apa untuk diblur.

## Komponen

- **Tombol:** primary isi indigo dengan teks putih, radius 6px, bobot 600. Secondary transparan dengan garis 1px, radius sama. Quiet tanpa garis, hanya perubahan warna teks saat hover. Destructive teks merah dengan garis merah. Ukuran: small 32px, medium 38px, large 44px. Hanya **satu tombol primary** per bagian layar.
- **Kartu:** permukaan putih, garis 1px halus, radius 8px, overflow terpotong. Saat hover naik 2px dan bertambah bayangan. Transisi 180ms.
- **Input:** garis 1px halus, permukaan putih, radius 6px, padding 10px 14px, ukuran teks 14px. Saat fokus border jadi aksen dengan ring indigo. Saat error border jadi merah. Placeholder memakai warna redup.
- **Chip:** bentuk pil (radius 9999px), latar netral, teks 12px bobot 600, padding 3px 12px. Status memakai warna semantik: hijau untuk selesai, kuning untuk ditinjau, merah untuk terhambat, aksen untuk sedang dikerjakan, netral untuk redup.
- **Daftar:** baris bertumpuk dengan garis rambut 1px, padding 12px 16px, saat hover latar halus berubah.
- **Checkbox:** 20px, radius penuh, latar permukaan-3 saat tidak terpilih, aksen saat terpilih.
- **Tabel:** header latar permukaan-2, baris dipisah garis rambut 1px, kolom angka memakai tabular figures.
- **Navigasi:** sidebar 264px, sticky, garis rambut 1px di kanan. Item aktif memakai blok tinta solid dengan teks terang. Bisa ditutup jadi rail ikon 64px di desktop, dan jadi drawer di bawah 1024px.

## Spasi dan radius

Satuan dasar 4px. Skala: 4, 8, 12, 16, 20, 24, 32, 40, 48.

- Padding komponen: kecil 8x12, sedang 10x16, besar 12x24.
- Konten maksimum 1280px dengan padding 24px (16px di ponsel).
- Radius: 4px chip kecil dan dot, 6px tombol/input/select, 8px panel dan kartu, 12px dialog, 9999px chip dan avatar.
- Breakpoint: 560 / 720 / 1024 / 1280.

## Tata letak

- Sidebar 264px, sticky, garis rambut 1px. Di desktop bisa ditutup jadi rail ikon 64px; di bawah 1024px jadi drawer.
- Info akun dan tombol keluar berada di sidebar, bukan topbar.
- Topbar berisi workspace switcher, chip peran, dan toggle tema.
- Konten maksimum 1280px, padding 24px (16px di ponsel).
- Tabel data berubah menjadi daftar kartu di bawah 720px lewat satu aturan CSS, bukan duplikasi markup.
- Breakpoint: 560 / 720 / 1024 / 1280.

## Gerak

Hanya transisi 120 sampai 180ms untuk hover, buka/tutup drawer, rail sidebar, dan masuknya dialog. Semua mati pada `prefers-reduced-motion: reduce`.

## Yang tidak dipakai

Gradient dekoratif, glassmorphism, latar grid, emoji sebagai ikon, kartu fitur seragam, ikon dari library yang terlihat seperti library itu, ukuran teks di luar skala, bobot selain 400 dan 600.

Catatan: "glow" dan "badge pil" ada di daftar larangan versi lama. Keduanya dipakai sekarang karena Genesis menetapkan glow indigo saat hover tombol primary dan chip berbentuk pil. Penggunaannya dibatasi: glow hanya pada tombol primary saat hover, pil hanya pada chip status dan peran.
