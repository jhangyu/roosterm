import 'package:flutter/material.dart';

import 'fluent_icon.dart';
import 'host_page.dart';
import 'hosts.dart';
import 'notice.dart';
import 'tokens.dart';

class HostListPage extends StatefulWidget {
  const HostListPage({super.key});

  @override
  State<HostListPage> createState() => _HostListPageState();
}

class _HostListPageState extends State<HostListPage> {
  final HostStore _store = HostStore();
  List<Host>? _hosts;
  Object? _loadError;

  @override
  void initState() {
    super.initState();
    _store.load().then(
          (hosts) => setState(() => _hosts = hosts),
          onError: (Object e) => setState(() => _loadError = e),
        );
  }

  Future<void> _replace(List<Host> hosts) async {
    try {
      await _store.save(hosts);
    } on Exception catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('Could not save the host list: $e')));
      return;
    }
    if (!mounted) return;
    setState(() => _hosts = hosts);
  }

  Future<void> _edit({Host? existing, int? index}) async {
    final host = await showDialog<Host>(
      context: context,
      builder: (_) => _HostDialog(existing: existing),
    );
    if (host == null) return; // dialog cancelled
    final hosts = [...?_hosts];
    if (index == null) {
      hosts.add(host);
    } else {
      hosts[index] = host;
    }
    await _replace(hosts);
  }

  Future<void> _delete(int index) async {
    final host = _hosts![index];
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('Delete "${host.name}"?'),
        content: const Text('The host is removed from this device only.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            style: TextButton.styleFrom(foregroundColor: mutedForeground),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: TextButton.styleFrom(foregroundColor: destructive),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (ok == true) await _replace([..._hosts!]..removeAt(index));
  }

  @override
  Widget build(BuildContext context) {
    final Widget body;
    if (_loadError != null) {
      body = Notice(
        icon: 'warning',
        iconColor: destructive,
        title: 'Could not read the host list',
        detail: '$_loadError',
      );
    } else if (_hosts == null) {
      body = const Center(child: Text('Loading…', style: TextStyle(color: mutedForeground)));
    } else if (_hosts!.isEmpty) {
      body = const Notice(
        icon: 'server',
        iconColor: mutedForeground,
        title: 'No hosts yet',
        detail: 'Add a Roosterm host to open its terminals.',
      );
    } else {
      body = ListView.separated(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 96),
        itemCount: _hosts!.length,
        separatorBuilder: (_, _) => const SizedBox(height: 8),
        itemBuilder: (context, i) {
          final h = _hosts![i];
          return _HostTile(
            host: h,
            onOpen: () => Navigator.push(
              context,
              MaterialPageRoute<void>(builder: (_) => HostPage(host: h)),
            ),
            onEdit: () => _edit(existing: h, index: i),
            onDelete: () => _delete(i),
          );
        },
      );
    }
    return Scaffold(
      appBar: AppBar(
        title: const Text('Roosterm', style: TextStyle(fontWeight: FontWeight.w600)),
      ),
      body: body,
      floatingActionButton: _hosts == null
          ? null
          : FloatingActionButton.extended(
              onPressed: () => _edit(),
              icon: const FluentIcon('add', color: Colors.white),
              label: const Text('Add host'),
            ),
    );
  }
}

class _HostTile extends StatelessWidget {
  const _HostTile({required this.host, required this.onOpen, required this.onEdit, required this.onDelete});

  final Host host;
  final VoidCallback onOpen;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) => Material(
        color: card,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(radiusMd),
          side: const BorderSide(color: border),
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(radiusMd),
          onTap: onOpen,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(12, 12, 4, 12),
            child: Row(children: [
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: brandTint,
                  borderRadius: BorderRadius.circular(radiusSm),
                ),
                alignment: Alignment.center,
                child: const FluentIcon('server', color: brandTintForeground),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text(host.name,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                  Text(host.url.toString(),
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 13, color: mutedForeground)),
                ]),
              ),
              IconButton(
                tooltip: 'Edit',
                onPressed: onEdit,
                icon: const FluentIcon('edit', color: mutedForeground),
              ),
              IconButton(
                tooltip: 'Delete',
                onPressed: onDelete,
                icon: const FluentIcon('delete', color: destructive),
              ),
            ]),
          ),
        ),
      );
}

class _HostDialog extends StatefulWidget {
  const _HostDialog({this.existing});

  final Host? existing;

  @override
  State<_HostDialog> createState() => _HostDialogState();
}

class _HostDialogState extends State<_HostDialog> {
  late final TextEditingController _name = TextEditingController(text: widget.existing?.name);
  late final TextEditingController _url = TextEditingController(text: widget.existing?.url.toString());
  String? _error;

  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    super.dispose();
  }

  void _save() {
    try {
      Navigator.pop(context, Host.parse(_name.text, _url.text));
    } on FormatException catch (e) {
      setState(() => _error = e.message);
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
        title: Text(widget.existing == null ? 'Add host' : 'Edit host'),
        content: Column(mainAxisSize: MainAxisSize.min, children: [
          TextField(controller: _name, decoration: const InputDecoration(labelText: 'Name')),
          const SizedBox(height: 16),
          TextField(
            controller: _url,
            keyboardType: TextInputType.url,
            decoration: InputDecoration(labelText: 'URL', hintText: 'https://', errorText: _error),
          ),
        ]),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            style: TextButton.styleFrom(foregroundColor: mutedForeground),
            child: const Text('Cancel'),
          ),
          FilledButton(onPressed: _save, child: const Text('Save')),
        ],
      );
}
