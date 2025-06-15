import 'package:flutter/material.dart';
import '../services/po_box_service.dart';

class SupervisorScreen extends StatefulWidget {
  @override
  _SupervisorScreenState createState() => _SupervisorScreenState();
}

class _SupervisorScreenState extends State<SupervisorScreen> {
  List<Map<String, dynamic>> poBoxLog = [];

  @override
  void initState() {
    super.initState();
    _loadPOBoxLog();
  }

  void _loadPOBoxLog() async {
    var log = await POBoxService.getPOBoxLog();
    setState(() {
      poBoxLog = log;
    });
  }

  void _editEntry(Map<String, dynamic> entry) async {
    final controller = TextEditingController(text: entry['address']);
    final newAddress = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Edit Address'),
        content: TextField(controller: controller),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, controller.text),
            child: Text('Save'),
          ),
        ],
      ),
    );

    if (newAddress != null) {
      await POBoxService.editPOBox(entry['id'], newAddress);
      _loadPOBoxLog();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text("Supervisor Panel")),
      body: ListView.builder(
        itemCount: poBoxLog.length,
        itemBuilder: (context, index) {
          final entry = poBoxLog[index];
          return ListTile(
            title: Text("Package ID: ${entry['packageId']}"),
            subtitle: Text("Address: ${entry['address']}"),
            trailing: IconButton(
              icon: Icon(Icons.edit),
              onPressed: () => _editEntry(entry),
            ),
          );
        },
      ),
    );
  }
}
