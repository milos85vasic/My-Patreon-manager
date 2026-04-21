import 'dart:convert';
import 'package:http/http.dart' as http;
import '../models/envwizard.dart';

class EnvWizardApi {
  final String baseUrl;
  final http.Client _client;

  EnvWizardApi({
    required this.baseUrl,
    http.Client? client,
  }) : _client = client ?? http.Client();

  Uri _uri(String path) => Uri.parse('$baseUrl$path');

  Future<List<Category>> getCategories() async {
    final resp = await _client.get(_uri('/api/categories'));
    _check(resp);
    final list = jsonDecode(resp.body) as List<dynamic>;
    return list.map((e) => Category.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<List<EnvVar>> getVars() async {
    final resp = await _client.get(_uri('/api/vars'));
    _check(resp);
    final list = jsonDecode(resp.body) as List<dynamic>;
    return list.map((e) => EnvVar.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Map<String, dynamic>> getVar(String name) async {
    final resp = await _client.get(_uri('/api/vars/$name'));
    _check(resp);
    return jsonDecode(resp.body) as Map<String, dynamic>;
  }

  Future<void> setValue(String name, String value) async {
    final resp = await _client.post(
      _uri('/api/vars/$name'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'value': value}),
    );
    _check(resp);
  }

  Future<void> skipVar(String name) async {
    final resp = await _client.post(_uri('/api/skip/$name'));
    _check(resp);
  }

  Future<WizardState> getWizardState() async {
    final resp = await _client.get(_uri('/api/wizard/state'));
    _check(resp);
    return WizardState.fromJson(jsonDecode(resp.body) as Map<String, dynamic>);
  }

  Future<void> next() async {
    final resp = await _client.post(_uri('/api/wizard/next'));
    _check(resp);
  }

  Future<void> prev() async {
    final resp = await _client.post(_uri('/api/wizard/prev'));
    _check(resp);
  }

  Future<String> save() async {
    final resp = await _client.post(_uri('/api/save'));
    _check(resp);
    final data = jsonDecode(resp.body) as Map<String, dynamic>;
    return data['content'] as String? ?? '';
  }

  Future<List<String>> listProfiles() async {
    final resp = await _client.get(_uri('/api/profiles'));
    _check(resp);
    final list = jsonDecode(resp.body) as List<dynamic>;
    return list.map((e) => e.toString()).toList();
  }

  Future<void> saveProfile(String name) async {
    final resp = await _client.post(
      _uri('/api/profiles'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'name': name}),
    );
    _check(resp);
  }

  void _check(http.Response resp) {
    if (resp.statusCode >= 400) {
      throw ApiException(resp.statusCode, resp.body);
    }
  }
}

class ApiException implements Exception {
  final int statusCode;
  final String body;

  ApiException(this.statusCode, this.body);

  @override
  String toString() => 'ApiException($statusCode): $body';
}
