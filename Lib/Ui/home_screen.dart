import 'package:flutter/material.dart';
import 'route_trace_screen.dart';
import 'scan_packages_screen.dart';
import 'supervisor_screen.dart';
import 'hazard_screen.dart';

class HomeScreen extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text("USPS LOT NAV"),
      ),
      body: Padding(
        padding: const EdgeInsets.all(24.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ElevatedButton(
              child: Text("Start New Route Trace"),
              onPressed: () {
                Navigator.push(
                  context,
                  MaterialPageRoute(builder: (context) => RouteTraceScreen()),
                );
              },
            ),
            ElevatedButton(
              child: Text("Scan Packages"),
              onPressed: () {
                Navigator.push(
                  context,
                  MaterialPageRoute(builder: (context) => ScanPackagesScreen()),
                );
              },
            ),
            ElevatedButton(
              child: Text("Supervisor/Admin"),
              onPressed: () {
                Navigator.push(
                  context,
                  MaterialPageRoute(builder: (context) => SupervisorScreen()),
                );
              },
            ),
            ElevatedButton(
              child: Text("Hazard Log"),
              onPressed: () {
                Navigator.push(
                  context,
                  MaterialPageRoute(builder: (context) => HazardScreen()),
                );
              },
            ),
          ],
        ),
      ),
    );
  }
}
