import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';

class GolemTheme {
  static const bgPrimary = Color(0xFF0d1117);
  static const bgSurface = Color(0xFF161b22);
  static const bgElevated = Color(0xFF1c2128);
  static const border = Color(0xFF30363d);
  static const textPrimary = Color(0xFFe6edf3);
  static const textSecondary = Color(0xFF8b949e);
  static const accent = Color(0xFF58a6ff);
  static const green = Color(0xFF3fb950);
  static const yellow = Color(0xFFd29922);
  static const red = Color(0xFFf85149);

  static ThemeData dark() {
    return ThemeData(
      useMaterial3: true,
      brightness: Brightness.dark,
      scaffoldBackgroundColor: bgPrimary,
      colorScheme: const ColorScheme.dark(
        primary: accent,
        surface: bgSurface,
        onSurface: textPrimary,
        onPrimary: Colors.white,
        outline: border,
      ),
      cardTheme: const CardThemeData(
        color: bgSurface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.all(Radius.circular(8)),
          side: BorderSide(color: border),
        ),
      ),
      dividerColor: border,
      textTheme: GoogleFonts.interTextTheme(ThemeData.dark().textTheme).apply(
        bodyColor: textPrimary,
        displayColor: textPrimary,
      ),
      appBarTheme: const AppBarTheme(
        backgroundColor: bgSurface,
        foregroundColor: textPrimary,
        elevation: 0,
        surfaceTintColor: Colors.transparent,
      ),
      tabBarTheme: const TabBarThemeData(
        labelColor: textPrimary,
        unselectedLabelColor: textSecondary,
        indicatorColor: accent,
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: bgPrimary,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: accent),
        ),
        contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        isDense: true,
      ),
      dialogTheme: const DialogThemeData(
        backgroundColor: bgSurface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.all(Radius.circular(12)),
          side: BorderSide(color: border),
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          backgroundColor: accent,
          foregroundColor: Colors.white,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(foregroundColor: textSecondary),
      ),
    );
  }

  static TextStyle monoStyle({double fontSize = 13}) {
    return GoogleFonts.jetBrainsMono(
      fontSize: fontSize,
      color: textPrimary,
      height: 1.5,
    );
  }
}
