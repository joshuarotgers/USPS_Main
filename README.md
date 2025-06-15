# USPS LOT NAV — Native Cross-Platform Build

---

## 🚀 System Overview

This is the full native Flutter build for USPS Line of Travel Navigation.

- ✅ 100% offline-first
- ✅ BYOD phones (iPhone/Android)
- ✅ Zebra industrial scanners (DataWedge support)
- ✅ Supervisor audit panels
- ✅ PO Box tracking
- ✅ Package scanning
- ✅ GPS route tracing
- ✅ Hazard logging
- ✅ SQLite secure local database

---

## ⚙ Tech Stack

| Layer | Technology |
|-------|-------------|
| UI | Flutter (Dart) |
| Offline DB | SQLite (`sqflite` plugin) |
| GPS | Geolocator |
| Scanning | Flutter Barcode Scanner (BYOD) + Zebra DataWedge (Zebra) |
| Native Plugins | Intent Channels for Zebra |
| Platform Targets | iPhone, Android, Zebra TC7X, TC2X |

---

## 📦 Folder Structure

```plaintext
/lib/
  main.dart
  /ui/
  /services/
  /zebra/
  /models/
  /assets/
  /data/
  /guides/
pubspec.yaml
README.md
