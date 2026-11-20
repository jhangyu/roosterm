import 'package:flutter/material.dart';
import 'package:webview_flutter/webview_flutter.dart';
import 'package:webview_flutter_android/webview_flutter_android.dart';

import 'fluent_icon.dart';
import 'hosts.dart';
import 'tokens.dart';

class HostPage extends StatefulWidget {
  const HostPage({super.key, required this.host});

  final Host host;

  @override
  State<HostPage> createState() => _HostPageState();
}

class _HostPageState extends State<HostPage> {
  late final WebViewController _controller;
  WebResourceError? _error;

  @override
  void initState() {
    super.initState();
    _controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(NavigationDelegate(
        onWebResourceError: (e) {
          // null = platform did not say; treat as main frame so the failure is visible.
          if (e.isForMainFrame ?? true) setState(() => _error = e);
        },
      ))
      ..loadRequest(widget.host.url);
  }

  // Hybrid composition (a real view in the hierarchy) instead of the surface
  // embedding, which made page animations stutter.
  Widget _webView() {
    final params = PlatformWebViewWidgetCreationParams(controller: _controller.platform);
    return WebViewWidget.fromPlatformCreationParams(
      params: AndroidWebViewWidgetCreationParams.fromPlatformWebViewWidgetCreationParams(
          params,
          displayWithHybridComposition: true),
    );
  }

  void _reload() {
    setState(() => _error = null);
    _controller.loadRequest(widget.host.url);
  }

  @override
  Widget build(BuildContext context) {
    final error = _error;
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) async {
        if (didPop) return;
        if (await _controller.canGoBack()) {
          await _controller.goBack();
        } else if (context.mounted) {
          Navigator.of(context).pop();
        }
      },
      child: Scaffold(
        body: SafeArea(
          child: error == null
              ? _webView()
              : Center(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Column(mainAxisSize: MainAxisSize.min, children: [
                      const FluentIcon('warning', color: destructive, size: 32),
                      const SizedBox(height: 12),
                      Text('Could not load ${widget.host.url}',
                          textAlign: TextAlign.center,
                          style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                      const SizedBox(height: 4),
                      Text('${error.errorCode}: ${error.description}',
                          textAlign: TextAlign.center,
                          style: const TextStyle(color: mutedForeground)),
                      const SizedBox(height: 16),
                      FilledButton.icon(
                        onPressed: _reload,
                        icon: const FluentIcon('arrow_clockwise', color: Colors.white),
                        label: const Text('Reload'),
                      ),
                    ]),
                  ),
                ),
        ),
      ),
    );
  }
}
