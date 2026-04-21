import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../main.dart';
import '../widgets/var_card.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  String? _selectedCategory;
  final _urlController = TextEditingController(text: 'http://localhost:8081');

  @override
  Widget build(BuildContext context) {
    return Consumer<WizardStateNotifier>(
      builder: (context, notifier, _) {
        if (notifier.loading) {
          return const Scaffold(
            body: Center(child: CircularProgressIndicator()),
          );
        }

        if (notifier.error != null) {
          return Scaffold(
            appBar: AppBar(title: const Text('EnvWizard')),
            body: Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const Icon(Icons.error_outline, size: 48, color: Color(0xFFE94560)),
                  const SizedBox(height: 16),
                  Text('Connection Error', style: Theme.of(context).textTheme.titleLarge),
                  const SizedBox(height: 8),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 32),
                    child: Text(notifier.error!, textAlign: TextAlign.center),
                  ),
                  const SizedBox(height: 24),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 32),
                    child: TextField(
                      controller: _urlController,
                      decoration: const InputDecoration(
                        labelText: 'Server URL',
                        hintText: 'http://localhost:8081',
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  ElevatedButton(
                    onPressed: () {
                      final notifier = context.read<WizardStateNotifier>();
                      notifier.setServerUrl(_urlController.text);
                      notifier.init();
                    },
                    child: const Text('Connect'),
                  ),
                ],
              ),
            ),
          );
        }

        final filteredVars = notifier.varsForCategory(_selectedCategory);
        final progress = notifier.state.progressPercent;

        return Scaffold(
          appBar: AppBar(
            title: const Text('EnvWizard'),
            actions: [
              IconButton(
                icon: const Icon(Icons.save),
                onPressed: () => _save(context, notifier),
                tooltip: 'Save .env',
              ),
              IconButton(
                icon: const Icon(Icons.folder),
                onPressed: () => _saveProfile(context, notifier),
                tooltip: 'Save Profile',
              ),
            ],
          ),
          body: Column(
            children: [
              Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  children: [
                    LinearProgressIndicator(
                      value: progress,
                      backgroundColor: const Color(0xFF16213E),
                      valueColor: const AlwaysStoppedAnimation<Color>(Color(0xFFE94560)),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      '${notifier.state.completed}/${notifier.state.totalSteps} configured',
                      style: const TextStyle(color: Colors.grey),
                    ),
                  ],
                ),
              ),
              SizedBox(
                height: 40,
                child: ListView(
                  scrollDirection: Axis.horizontal,
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  children: [
                    _categoryChip(null, 'All', notifier),
                    ...notifier.categories.map((c) => _categoryChip(c.id, c.name, notifier)),
                  ],
                ),
              ),
              const SizedBox(height: 8),
              Expanded(
                child: ListView.builder(
                  padding: const EdgeInsets.all(16),
                  itemCount: filteredVars.length,
                  itemBuilder: (context, index) {
                    return VarCard(envVar: filteredVars[index]);
                  },
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _categoryChip(String? id, String label, WizardStateNotifier notifier) {
    final selected = _selectedCategory == id;
    return Padding(
      padding: const EdgeInsets.only(right: 8),
      child: ChoiceChip(
        label: Text(label),
        selected: selected,
        onSelected: (_) => setState(() => _selectedCategory = selected ? null : id),
        selectedColor: const Color(0xFF0F3460),
        side: BorderSide(
          color: selected ? const Color(0xFFE94560) : const Color(0xFF0F3460),
        ),
      ),
    );
  }

  Future<void> _save(BuildContext context, WizardStateNotifier notifier) async {
    try {
      final content = await notifier.save();
      if (!context.mounted) return;
      showDialog(
        context: context,
        builder: (ctx) => AlertDialog(
          title: const Text('Generated .env'),
          content: SizedBox(
            width: double.maxFinite,
            child: TextField(
              maxLines: 20,
              readOnly: true,
              controller: TextEditingController(text: content),
              style: const TextStyle(fontFamily: 'monospace', fontSize: 12),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('Close'),
            ),
          ],
        ),
      );
    } catch (e) {
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Save failed: $e')),
      );
    }
  }

  Future<void> _saveProfile(BuildContext context, WizardStateNotifier notifier) async {
    final name = await showDialog<String>(
      context: context,
      builder: (ctx) {
        final controller = TextEditingController();
        return AlertDialog(
          title: const Text('Save Profile'),
          content: TextField(
            controller: controller,
            decoration: const InputDecoration(hintText: 'Profile name'),
            autofocus: true,
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
            ElevatedButton(
              onPressed: () => Navigator.pop(ctx, controller.text),
              child: const Text('Save'),
            ),
          ],
        );
      },
    );
    if (name != null && name.isNotEmpty) {
      await notifier.saveProfile(name);
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Profile "$name" saved')),
        );
      }
    }
  }
}
