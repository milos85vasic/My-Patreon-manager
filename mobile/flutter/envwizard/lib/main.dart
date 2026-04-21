import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:sentry_flutter/sentry_flutter.dart';
import 'models/envwizard.dart';
import 'services/api.dart';
import 'screens/home_screen.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  await SentryFlutter.init(
    (options) {
      options.dsn = const String.fromEnvironment('SENTRY_DSN', defaultValue: '');
      options.tracesSampleRate = 1.0;
    },
    appRunner: () => runApp(
      ChangeNotifierProvider(
        create: (_) => WizardStateNotifier(
          EnvWizardApi(baseUrl: const String.fromEnvironment(
            'API_URL',
            defaultValue: 'http://localhost:8081',
          )),
        )..init(),
        child: const EnvWizardApp(),
      ),
    ),
  );
}

class EnvWizardApp extends StatelessWidget {
  const EnvWizardApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'EnvWizard',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        brightness: Brightness.dark,
        primarySwatch: Colors.deepPurple,
        scaffoldBackgroundColor: const Color(0xFF1A1A2E),
        colorScheme: const ColorScheme.dark(
          primary: Color(0xFFE94560),
          secondary: Color(0xFF0F3460),
          surface: Color(0xFF16213E),
        ),
        inputDecorationTheme: const InputDecorationTheme(
          filled: true,
          fillColor: Color(0xFF1A1A2E),
          border: OutlineInputBorder(
            borderSide: BorderSide(color: Color(0xFF0F3460)),
          ),
          focusedBorder: OutlineInputBorder(
            borderSide: BorderSide(color: Color(0xFFE94560)),
          ),
        ),
      ),
      home: const HomeScreen(),
    );
  }
}

class WizardStateNotifier extends ChangeNotifier {
  final EnvWizardApi _api;

  List<Category> _categories = [];
  List<EnvVar> _vars = [];
  WizardState _state = const WizardState();
  Map<String, String> _values = {};
  Map<String, bool> _skipped = {};
  Map<String, String> _errors = {};
  bool _loading = true;
  String? _error;
  String? _serverUrl;

  WizardStateNotifier(this._api);

  List<Category> get categories => _categories;
  List<EnvVar> get vars => _vars;
  WizardState get state => _state;
  Map<String, String> get values => _values;
  Map<String, bool> get skipped => _skipped;
  Map<String, String> get errors => _errors;
  bool get loading => _loading;
  String? get error => _error;
  String? get serverUrl => _serverUrl;

  void setServerUrl(String url) {
    _serverUrl = url;
    notifyListeners();
  }

  Future<void> init() async {
    _loading = true;
    _error = null;
    notifyListeners();

    try {
      final results = await Future.wait([
        _api.getCategories(),
        _api.getVars(),
        _api.getWizardState(),
      ]);
      _categories = results[0] as List<Category>;
      _vars = results[1] as List<EnvVar>;
      _state = results[2] as WizardState;
      _loading = false;
    } catch (e) {
      _error = e.toString();
      _loading = false;
    }
    notifyListeners();
  }

  Future<String?> setValue(String name, String value) async {
    try {
      await _api.setValue(name, value);
      _values[name] = value;
      _skipped.remove(name);
      _errors.remove(name);
      await _refreshState();
      return null;
    } on ApiException catch (e) {
      _errors[name] = e.body;
      notifyListeners();
      return e.body;
    }
  }

  Future<void> skipVar(String name) async {
    await _api.skipVar(name);
    _skipped[name] = true;
    _values.remove(name);
    _errors.remove(name);
    await _refreshState();
  }

  Future<String> save() async {
    return _api.save();
  }

  Future<void> saveProfile(String name) async {
    await _api.saveProfile(name);
  }

  Future<void> _refreshState() async {
    try {
      _state = await _api.getWizardState();
    } catch (_) {}
    notifyListeners();
  }

  List<EnvVar> varsForCategory(String? categoryId) {
    if (categoryId == null) return _vars;
    return _vars.where((v) => v.category?.id == categoryId).toList();
  }

  bool isSet(String name) => _values.containsKey(name);
}
