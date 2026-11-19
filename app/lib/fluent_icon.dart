import 'package:flutter/widgets.dart';
import 'package:flutter_svg/flutter_svg.dart';

/// Fluent 2 regular-20 icon from `assets/icons/name.svg`, tinted with [color].
class FluentIcon extends StatelessWidget {
  const FluentIcon(this.name, {super.key, required this.color, this.size = 20});

  final String name;
  final Color color;
  final double size;

  @override
  Widget build(BuildContext context) => SvgPicture.asset(
        'assets/icons/$name.svg',
        width: size,
        height: size,
        colorFilter: ColorFilter.mode(color, BlendMode.srcIn),
      );
}
