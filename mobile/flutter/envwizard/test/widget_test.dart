import 'package:flutter_test/flutter_test.dart';
import 'package:envwizard/models/envwizard.dart';

void main() {
  test('Category fromJson', () {
    final cat = Category.fromJson({
      'ID': 'server',
      'Name': 'Server',
      'Description': 'Server config',
      'Order': 1,
    });
    expect(cat.id, 'server');
    expect(cat.name, 'Server');
    expect(cat.order, 1);
  });

  test('EnvVar fromJson', () {
    final v = EnvVar.fromJson({
      'Name': 'PORT',
      'Description': 'HTTP port',
      'Required': true,
      'Secret': false,
      'Default': '8080',
      'CanGenerate': false,
    });
    expect(v.name, 'PORT');
    expect(v.required, true);
    expect(v.defaultValue, '8080');
    expect(v.secret, false);
  });

  test('WizardState fromJson', () {
    final state = WizardState.fromJson({
      'step': 5,
      'totalSteps': 64,
      'completed': 10,
      'missingRequired': ['PORT', 'ADMIN_KEY'],
      'hasErrors': false,
    });
    expect(state.step, 5);
    expect(state.totalSteps, 64);
    expect(state.completed, 10);
    expect(state.missingRequired.length, 2);
    expect(state.progressPercent, closeTo(0.156, 0.01));
  });

  test('WizardState progressPercent handles zero total', () {
    final state = WizardState.fromJson({});
    expect(state.progressPercent, 0.0);
  });
}
