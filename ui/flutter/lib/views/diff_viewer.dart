import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/graph.dart';
import '../providers/graph.dart';
import '../theme.dart';

/// A reusable diff viewer that shows an expandable file list with
/// syntax-highlighted patches. Use [compact] mode for sidebar embedding.
class DiffViewer extends ConsumerStatefulWidget {
  final bool compact;

  const DiffViewer({super.key, this.compact = false});

  @override
  ConsumerState<DiffViewer> createState() => _DiffViewerState();
}

class _DiffViewerState extends ConsumerState<DiffViewer> {
  final Set<String> _expandedFiles = {};
  final Map<String, String> _loadedPatches = {};
  final Set<String> _loadingFiles = {};

  Future<void> _toggleFile(FileDiff file) async {
    final path = file.path;

    if (_expandedFiles.contains(path)) {
      setState(() => _expandedFiles.remove(path));
      return;
    }

    setState(() => _expandedFiles.add(path));

    // If patch is already available inline or cached, nothing to load.
    if (file.patch != null || _loadedPatches.containsKey(path)) return;

    // Load patch from API.
    setState(() => _loadingFiles.add(path));
    try {
      final diffNotifier = ref.read(diffProvider.notifier);
      final patch = await diffNotifier.loadPatch(path);
      if (mounted) {
        setState(() {
          _loadedPatches[path] = patch;
          _loadingFiles.remove(path);
        });
      }
    } catch (_) {
      if (mounted) {
        setState(() => _loadingFiles.remove(path));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final diff = ref.watch(diffProvider);
    final pad = widget.compact ? 8.0 : 12.0;

    if (diff == null || diff.files.isEmpty) {
      return Padding(
        padding: EdgeInsets.all(pad),
        child: Text(
          'No changes since last run',
          style: const TextStyle(
            fontSize: 12,
            color: GolemTheme.textSecondary,
          ),
        ),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final file in diff.files) _buildFileEntry(file, pad),
      ],
    );
  }

  Widget _buildFileEntry(FileDiff file, double pad) {
    final isExpanded = _expandedFiles.contains(file.path);
    final isLoading = _loadingFiles.contains(file.path);
    final patch = file.patch ?? _loadedPatches[file.path];
    final fontSize = widget.compact ? 11.0 : 12.0;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        InkWell(
          onTap: () => _toggleFile(file),
          borderRadius: BorderRadius.circular(4),
          child: Padding(
            padding: EdgeInsets.symmetric(vertical: 4, horizontal: pad),
            child: Row(
              children: [
                Icon(
                  isExpanded
                      ? Icons.keyboard_arrow_down
                      : Icons.keyboard_arrow_right,
                  size: 16,
                  color: GolemTheme.textSecondary,
                ),
                const SizedBox(width: 4),
                Expanded(
                  child: Text(
                    file.path,
                    style: GolemTheme.monoStyle(fontSize: fontSize),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                const SizedBox(width: 8),
                if (file.additions > 0)
                  Text(
                    '+${file.additions}',
                    style: TextStyle(
                      fontSize: fontSize,
                      color: GolemTheme.green,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                if (file.additions > 0 && file.deletions > 0)
                  const SizedBox(width: 4),
                if (file.deletions > 0)
                  Text(
                    '-${file.deletions}',
                    style: TextStyle(
                      fontSize: fontSize,
                      color: GolemTheme.red,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
              ],
            ),
          ),
        ),
        if (isExpanded) ...[
          if (isLoading)
            Padding(
              padding: EdgeInsets.only(left: pad + 20, bottom: 8),
              child: const SizedBox(
                width: 14,
                height: 14,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  color: GolemTheme.accent,
                ),
              ),
            )
          else if (patch != null && patch.isNotEmpty)
            _PatchBlock(patch: patch, compact: widget.compact)
          else
            Padding(
              padding: EdgeInsets.only(left: pad + 20, bottom: 8),
              child: const Text(
                'No patch data available',
                style: TextStyle(
                  fontSize: 11,
                  color: GolemTheme.textSecondary,
                ),
              ),
            ),
        ],
        const Divider(height: 1, color: GolemTheme.border),
      ],
    );
  }
}

class _PatchBlock extends StatelessWidget {
  final String patch;
  final bool compact;

  const _PatchBlock({required this.patch, required this.compact});

  @override
  Widget build(BuildContext context) {
    final lines = patch.split('\n');
    final monoSize = compact ? 10.0 : 11.0;
    final hPad = compact ? 8.0 : 12.0;

    return Container(
      margin: EdgeInsets.only(left: hPad, right: hPad, bottom: 8),
      decoration: BoxDecoration(
        color: GolemTheme.bgPrimary,
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: GolemTheme.border),
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(4),
        child: SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: ConstrainedBox(
            constraints: const BoxConstraints(minWidth: 200),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final line in lines) _buildLine(line, monoSize, hPad),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildLine(String line, double fontSize, double hPad) {
    final Color color;
    final Color bgColor;

    if (line.startsWith('+')) {
      color = GolemTheme.green;
      bgColor = GolemTheme.green.withValues(alpha: 0.08);
    } else if (line.startsWith('-')) {
      color = GolemTheme.red;
      bgColor = GolemTheme.red.withValues(alpha: 0.08);
    } else if (line.startsWith('@@')) {
      color = GolemTheme.accent;
      bgColor = GolemTheme.accent.withValues(alpha: 0.06);
    } else {
      color = GolemTheme.textSecondary;
      bgColor = Colors.transparent;
    }

    return Container(
      width: double.infinity,
      color: bgColor,
      padding: EdgeInsets.symmetric(horizontal: hPad, vertical: 1),
      child: Text(
        line,
        style: GolemTheme.monoStyle(fontSize: fontSize).copyWith(color: color),
      ),
    );
  }
}
