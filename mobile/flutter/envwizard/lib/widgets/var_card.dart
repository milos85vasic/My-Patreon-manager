import 'dart:math';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../main.dart';
import '../models/envwizard.dart';

class VarCard extends StatefulWidget {
  final EnvVar envVar;

  const VarCard({super.key, required this.envVar});

  @override
  State<VarCard> createState() => _VarCardState();
}

class _VarCardState extends State<VarCard> {
  late TextEditingController _controller;
  bool _obscureText = true;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final notifier = context.watch<WizardStateNotifier>();
    final v = widget.envVar;
    final error = notifier.errors[v.name];
    final isSet = notifier.isSet(v.name);
    final isSkipped = notifier.skipped[v.name] ?? false;

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      color: const Color(0xFF16213E),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(8),
        side: BorderSide(
          color: v.required
              ? const Color(0xFFE94560)
              : v.secret
                  ? const Color(0xFF533483)
                  : const Color(0xFF0F3460),
          width: v.required ? 2 : 1,
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    v.name,
                    style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 16),
                  ),
                ),
                if (isSkipped)
                  const Chip(
                    label: Text('Skipped', style: TextStyle(fontSize: 11)),
                    visualDensity: VisualDensity.compact,
                  ),
              ],
            ),
            if (v.description.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(v.description, style: const TextStyle(color: Colors.grey)),
            ],
            const SizedBox(height: 8),
            Wrap(
              spacing: 4,
              children: [
                if (v.required)
                  _badge('Required', const Color(0xFFE94560)),
                if (v.secret)
                  _badge('Secret', const Color(0xFF533483)),
                if (v.defaultValue.isNotEmpty)
                  _badge('Default: ${v.defaultValue}', const Color(0xFF0F3460)),
                if (v.canGenerate)
                  _badge('Auto-generate', const Color(0xFF1A8F3C)),
              ],
            ),
            if (v.url.isNotEmpty) ...[
              const SizedBox(height: 4),
              InkWell(
                child: Text(v.url,
                    style: const TextStyle(
                        color: Colors.blue, decoration: TextDecoration.underline, fontSize: 12)),
                onTap: () {/* Could launch URL */},
              ),
            ],
            const SizedBox(height: 8),
            TextField(
              controller: _controller,
              obscureText: v.secret && _obscureText,
              decoration: InputDecoration(
                hintText: isSet ? 'Currently set' : 'Enter value...',
                suffixIcon: v.secret
                    ? IconButton(
                        icon: Icon(_obscureText ? Icons.visibility : Icons.visibility_off),
                        onPressed: () => setState(() => _obscureText = !_obscureText),
                      )
                    : null,
                errorText: error,
              ),
              onSubmitted: (val) => _setValue(notifier, val),
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                ElevatedButton(
                  onPressed: () => _setValue(notifier, _controller.text),
                  style: ElevatedButton.styleFrom(backgroundColor: const Color(0xFFE94560)),
                  child: const Text('Set'),
                ),
                if (v.canGenerate) ...[
                  const SizedBox(width: 8),
                  OutlinedButton(
                    onPressed: () => _generate(notifier),
                    child: const Text('Generate'),
                  ),
                ],
                const SizedBox(width: 8),
                TextButton(
                  onPressed: () => notifier.skipVar(v.name),
                  style: TextButton.styleFrom(foregroundColor: Colors.grey),
                  child: const Text('Skip'),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _badge(String text, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(color: color, borderRadius: BorderRadius.circular(4)),
      child: Text(text, style: const TextStyle(fontSize: 11)),
    );
  }

  Future<void> _setValue(WizardStateNotifier notifier, String value) async {
    await notifier.setValue(widget.envVar.name, value);
  }

  Future<void> _generate(WizardStateNotifier notifier) async {
    final random = Random.secure();
    final hex = List.generate(32, (_) => random.nextInt(16).toRadixString(16)).join();
    _controller.text = hex;
    await notifier.setValue(widget.envVar.name, hex);
  }
}
