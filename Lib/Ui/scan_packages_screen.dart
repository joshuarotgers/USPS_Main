import 'package:flutter/material.dart';
import 'package:flutter_barcode_scanner/flutter_barcode_scanner.dart';
import '../services/local_db_service.dart';

class ScanPackagesScreen extends StatefulWidget {
  @override
  _ScanPackagesScreenState createState() => _ScanPackagesScreenState();
}

class _ScanPackagesScreenState extends State<ScanPackagesScreen> {
  List<String> scannedPackages = [];

  Future<void> _scanBarcode() async {
    String barcodeScanRes = await FlutterBarcodeScanner.scanBarcode(
      '#ff6666', 'Cancel', true, ScanMode.BARCODE);

    if (barcodeScanRes != '-1') {
      setState(() {
        scannedPackages.add(barcodeScanRes);
      });

      // Save scanned package to local DB
      LocalDBService.savePackage(barcodeScanRes, DateTime.now());
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text("Scan Packages")),
      body: Column(
        children: [
          ElevatedButton(
            onPressed: _scanBarcode,
            child: Text("Start Scan"),
          ),
          Expanded(
            child: ListView.builder(
              itemCount: scannedPackages.length,
              itemBuilder: (context, index) {
                return ListTile(
                  title: Text("Package: ${scannedPackages[index]}"),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}
