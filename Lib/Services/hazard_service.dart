import 'local_db_service.dart';

class HazardService {
  static Future<void> saveHazard(String note) async {
    await LocalDBService.saveHazard(note);
  }

  static Future<List<Map<String, dynamic>>> getHazardLog() async {
    final db = await LocalDBService.database;
    return await db.query('hazards', orderBy: 'timestamp DESC');
  }
}
