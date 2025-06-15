import 'package:flutter/material.dart';
import 'ui/home_screen.dart';

void main() {
  runApp(USPSLotNavApp());
}

class USPSLotNavApp extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'USPS LOT NAV',
      theme: ThemeData(
        primarySwatch: Colors.blue,
      ),
      home: HomeScreen(),
      debugShowCheckedModeBanner: false,
    );
  }
}
