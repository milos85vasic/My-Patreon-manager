class Category {
  final String id;
  final String name;
  final String description;
  final int order;

  const Category({
    required this.id,
    required this.name,
    required this.description,
    required this.order,
  });

  factory Category.fromJson(Map<String, dynamic> json) {
    return Category(
      id: json['ID'] as String? ?? '',
      name: json['Name'] as String? ?? '',
      description: json['Description'] as String? ?? '',
      order: json['Order'] as int? ?? 0,
    );
  }
}

class EnvVar {
  final String name;
  final String description;
  final Category? category;
  final bool required;
  final String defaultValue;
  final String validation;
  final String validationRule;
  final String url;
  final bool canGenerate;
  final bool secret;
  final String example;

  const EnvVar({
    required this.name,
    this.description = '',
    this.category,
    this.required = false,
    this.defaultValue = '',
    this.validation = '',
    this.validationRule = '',
    this.url = '',
    this.canGenerate = false,
    this.secret = false,
    this.example = '',
  });

  factory EnvVar.fromJson(Map<String, dynamic> json) {
    return EnvVar(
      name: json['Name'] as String? ?? '',
      description: json['Description'] as String? ?? '',
      category: json['Category'] != null
          ? Category.fromJson(json['Category'] as Map<String, dynamic>)
          : null,
      required: json['Required'] as bool? ?? false,
      defaultValue: json['Default'] as String? ?? '',
      validation: json['Validation'] as String? ?? '',
      validationRule: json['ValidationRule'] as String? ?? '',
      url: json['URL'] as String? ?? '',
      canGenerate: json['CanGenerate'] as bool? ?? false,
      secret: json['Secret'] as bool? ?? false,
      example: json['Example'] as String? ?? '',
    );
  }
}

class WizardState {
  final int step;
  final int totalSteps;
  final int completed;
  final List<String> missingRequired;
  final bool hasErrors;

  const WizardState({
    this.step = 0,
    this.totalSteps = 0,
    this.completed = 0,
    this.missingRequired = const [],
    this.hasErrors = false,
  });

  double get progressPercent =>
      totalSteps > 0 ? completed / totalSteps : 0.0;

  factory WizardState.fromJson(Map<String, dynamic> json) {
    return WizardState(
      step: json['step'] as int? ?? 0,
      totalSteps: json['totalSteps'] as int? ?? 0,
      completed: json['completed'] as int? ?? 0,
      missingRequired: (json['missingRequired'] as List<dynamic>?)
              ?.map((e) => e.toString())
              .toList() ??
          [],
      hasErrors: json['hasErrors'] as bool? ?? false,
    );
  }
}
