import 'package:flutter/material.dart';

import 'host_list_page.dart';
import 'tokens.dart';

void main() => runApp(const RoostermApp());

class RoostermApp extends StatelessWidget {
  const RoostermApp({super.key});

  @override
  Widget build(BuildContext context) => MaterialApp(
        title: 'Roosterm',
        theme: buildTheme(),
        home: const HostListPage(),
      );
}
