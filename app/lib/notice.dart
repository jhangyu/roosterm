import 'package:flutter/material.dart';

import 'fluent_icon.dart';
import 'tokens.dart';

class Notice extends StatelessWidget {
  const Notice({
    super.key,
    required this.icon,
    required this.iconColor,
    required this.title,
    required this.detail,
    this.action,
  });

  final String icon;
  final Color iconColor;
  final String title;
  final String detail;
  final Widget? action;

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            FluentIcon(icon, color: iconColor, size: 32),
            const SizedBox(height: 12),
            Text(title,
                textAlign: TextAlign.center,
                style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            Text(detail, textAlign: TextAlign.center, style: const TextStyle(color: mutedForeground)),
            if (action != null) ...[const SizedBox(height: 16), action!],
          ]),
        ),
      );
}
