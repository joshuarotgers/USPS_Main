import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import '../services/local_db_service.dart';

class RouteTraceScreen extends StatefulWidget {
  @override
  _RouteTraceScreenState createState() => _RouteTraceScreenState();
}

class _RouteTraceScreenState extends State<RouteTraceScreen> {
  bool _isTracking = false;
  Stream<Position>? _positionStream;

  void _startGPS() async {
    bool serviceEnabled = await Geolocator.isLocationServiceEnabled();
    if (!serviceEnabled) {
      await Geolocator.openLocationSettings();
      return;
    }

    LocationPermission permission = await Geolocator.checkPermission();
    if (permission == LocationPermission.denied) {
      permission = await Geolocator.requestPermission();
    }

    if (permission == LocationPermission.deniedForever) {
      return;
    }

    setState(() => _isTracking = true);
    _positionStream = Geolocator.getPositionStream(
      locationSettings: LocationSettings(accuracy: LocationAccuracy.high),
    );

    _positionStream!.listen((Position pos) {
      print('GPS: ${pos.latitude}, ${pos.longitude}');
      LocalDBService.saveRoutePoint(pos.latitude, pos.longitude, DateTime.now());
    });
  }

  void _stopGPS() {
    setState(() => _isTracking = false);
    _positionStream = null;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text("Route Trace")),
      body: Center(
        child: _isTracking
            ? Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Text("Recording GPS route..."),
                  SizedBox(height: 20),
                  ElevatedButton(
                    onPressed: _stopGPS,
                    child: Text("Stop Tracking"),
                  ),
                ],
              )
            : ElevatedButton(
                onPressed: _startGPS,
                child: Text("Start Route Trace"),
              ),
      ),
    );
  }
}
