import 'package:flutter/material.dart';
import '../services/hazard_service.dart';

class HazardScreen extends StatefulWidget {
  @override
  _HazardScreenState createState() => _HazardScreenState();
}

class _HazardScreenState extends State<HazardScreen> {
  List<Map<String, dynamic>> hazards = [];

  @override
  void initState() {
    super.initState();
    _loadHazards();
  }

  void _loadHazards() async {
    var log = await HazardService.getHazardLog();
    setState(() {
      hazards = log;
    });
  }

  void _addHazard() async {
    final TextEditingController controller = TextEditingController();
    final hazard = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Log Hazard'),
        content: TextField(controller: controller, decoration: InputDecoration(hintText: "Enter hazard notes")),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, controller.text),
            child: Text('Save'),
          ),
        ],
      ),
    );

    if (hazard != null && hazard.isNotEmpty) {
      await HazardService.saveHazard(hazard);
      _loadHazards();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text("Hazard Log")),
      floatingActionButton: FloatingActionButton(
        onPressed: _addHazard,
        child: Icon(Icons.add),
      ),
      body: ListView.builder(
        itemCount: hazards.length,
        itemBuilder: (context, index) {
          final entry = hazards[index];
          return ListTile(
            title: Text(entry['note']),
            subtitle: Text("Logged: ${entry['timestamp']}"),
          );
        },
      ),
    );
  }
}
