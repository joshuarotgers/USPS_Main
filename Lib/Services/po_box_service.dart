import 'local_db_service.dart';

class POBoxService {
  static Future<List<Map<String, dynamic>>> getPOBoxLog() async {
    final db = await LocalDBService.database;
    return await db.query('po_boxes', orderBy: 'id DESC');
  }

  static Future<void> editPOBox(int id, String newAddress) async {
    final db = await LocalDBService.database;
    await db.update(
      'po_boxes',
      {'address': newAddress, 'edited': 1},
      where: 'id = ?',
      whereArgs: [id],
    );
  }
}
