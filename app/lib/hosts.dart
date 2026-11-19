import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

class Host {
  const Host(this.name, this.url);

  final String name;
  final Uri url;

  /// Boundary parse. https only: the session cookie is Secure, an http host would loop on login.
  static Host parse(String name, String url) {
    final n = name.trim();
    if (n.isEmpty) {
      throw const FormatException('Name: expected a non-empty string, got an empty one');
    }
    final u = Uri.tryParse(url.trim());
    if (u == null || u.scheme != 'https' || u.host.isEmpty) {
      throw FormatException('URL: expected https://<host>[:port][/path], got "$url"');
    }
    return Host(n, u);
  }

  Map<String, String> toJson() => {'name': name, 'url': url.toString()};
}

class HostStore {
  static const _key = 'hosts';
  final SharedPreferencesAsync _prefs = SharedPreferencesAsync();

  Future<List<Host>> load() async {
    final raw = await _prefs.getString(_key);
    if (raw == null) return []; // first launch: nothing saved yet
    final entries = jsonDecode(raw) as List<dynamic>;
    return [
      for (final e in entries)
        Host.parse((e as Map<String, dynamic>)['name'] as String, e['url'] as String),
    ];
  }

  Future<void> save(List<Host> hosts) =>
      _prefs.setString(_key, jsonEncode([for (final h in hosts) h.toJson()]));
}
