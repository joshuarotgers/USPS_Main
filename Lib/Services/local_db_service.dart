import 'package:sqflite/sqflite.dart';
import 'package:path/path.dart';

class LocalDBService {
  static Database? _db;

  static Future<Database> get database async {
    if (_db != null) return _db!;
    _db = await _initDB();
    return _db!;
  }

  static Future<Database> _initDB() async {
    final path = join(await getDatabasesPath(), 'usps_lot_nav.db');
    return await openDatabase(path, version: 1, onCreate: _onCreate);
  }

  static Future _onCreate(Database db, int version) async {
    await db.execute('''
      CREATE TABLE routes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        latitude REAL,
        longitude REAL,
        timestamp TEXT
      )
    ''');

    await db.execute('''
      CREATE TABLE packages (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        barcode TEXT,
        scannedAt TEXT
      )
    ''');

    await db.execute('''
      CREATE TABLE po_boxes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        packageId TEXT,
        address TEXT,
        edited INTEGER DEFAULT 0
      )
    ''');

    await db.execute('''
      CREATE TABLE hazards (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        note TEXT,
        timestamp TEXT
      )
    ''');
  }

  // Route GPS trace
  static Future<void> saveRoutePoint(double lat, double lon, DateTime time) async {
    final db = await database;
    await db.insert('routes', {
      'latitude': lat,
      'longitude': lon,
      'timestamp': time.toIso8601String(),
    });
  }

  // Package scans
  static Future<void> savePackage(String barcode, DateTime time) async {
    final db = await database;
    await db.insert('packages', {
      'barcode': barcode,
      'scannedAt': time.toIso8601String(),
    });

    // PO Box logic: simulate detection (you will replace this later with real barcode parsing logic)
    if (barcode.contains("POBOX")) {
      await savePOBox(barcode, "Unassigned");
    }
  }

  // PO Box log
  static Future<void> savePOBox(String pkgId, String address) async {
    final db = await database;
    await db.insert('po_boxes', {
      'packageId': pkgId,
      'address': address,
    });
  }

  static Future<List<Map<String, dynamic>>> getPOBoxes() async {
    final db = await database;
    return await db.query('po_boxes');
  }

  static Future<void> editPOBox(int id, String newAddress) async {
    final db = await database;
    await db.update('po_boxes', {
      'address': newAddress,
      'edited': 1,
    }, where: 'id = ?', whereArgs: [id]);
  }

  // Hazard logs
  static Future<void> saveHazard(String note) async {
    final db = await database;
    await db.insert('hazards', {
      'note': note,
      'timestamp': DateTime.now().toIso8601String(),
    });
  }

  static Future<List<Map<String, dynamic>>> getHazards() async {
    final db = await database;
    return await db.query('hazards', orderBy: 'timestamp DESC');
  }
}
