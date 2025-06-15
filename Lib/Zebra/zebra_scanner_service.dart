// Zebra Scanner Service (basic intent listener stub)

import 'dart:async';
import 'package:flutter/services.dart';

class ZebraScannerService {
  static const EventChannel _dataWedgeChannel = EventChannel('com.zebra.datawedge.barcode');

  static Stream<String> get barcodeStream =>
      _dataWedgeChannel.receiveBroadcastStream().map((event) => event.toString());
}
