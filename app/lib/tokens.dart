import 'package:flutter/material.dart';

// Values from docs/mockups/3-calm-surfaces.html :root (Fluent 2 colors, Material 3 shape scale).
const Color brand = Color(0xFF0F6CBD);
const Color brandTint = Color(0xFFEBF3FC);
const Color brandTintForeground = Color(0xFF0F548C);
const Color background = Color(0xFFF3F3F3);
const Color foreground = Color(0xFF242424);
const Color card = Color(0xFFFFFFFF);
const Color mutedForeground = Color(0xFF616161);
const Color border = Color(0xFFE0E0E0);
const Color input = Color(0xFFD1D1D1);
const Color destructive = Color(0xFFB10E1C);

const double radiusSm = 8;
const double radiusMd = 12;
const double radiusXl = 28;

ThemeData buildTheme() {
  final scheme = ColorScheme.fromSeed(seedColor: brand, brightness: Brightness.light).copyWith(
    primary: brand,
    onPrimary: Colors.white,
    surface: card,
    onSurface: foreground,
    error: destructive,
    outline: input,
    outlineVariant: border,
  );
  final inputBorder = OutlineInputBorder(
    borderRadius: BorderRadius.circular(radiusSm),
    borderSide: const BorderSide(color: input),
  );
  return ThemeData(
    colorScheme: scheme,
    scaffoldBackgroundColor: background,
    appBarTheme: const AppBarTheme(
      backgroundColor: background,
      foregroundColor: foreground,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: card,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(radiusXl)),
    ),
    inputDecorationTheme: InputDecorationTheme(
      border: inputBorder,
      enabledBorder: inputBorder,
      focusedBorder: inputBorder.copyWith(borderSide: const BorderSide(color: brand, width: 2)),
      errorBorder: inputBorder.copyWith(borderSide: const BorderSide(color: destructive)),
      focusedErrorBorder:
          inputBorder.copyWith(borderSide: const BorderSide(color: destructive, width: 2)),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: brand,
        foregroundColor: Colors.white,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(radiusSm)),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: brand,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(radiusSm)),
      ),
    ),
    floatingActionButtonTheme: FloatingActionButtonThemeData(
      backgroundColor: brand,
      foregroundColor: Colors.white,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(radiusMd)),
    ),
  );
}
