import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/graph.dart';
import '../providers/connection.dart';
import '../providers/graph.dart';
import '../providers/project.dart';
import '../theme.dart';

class GraphExplorer extends ConsumerStatefulWidget {
  const GraphExplorer({super.key});

  @override
  ConsumerState<GraphExplorer> createState() => _GraphExplorerState();
}

class _GraphExplorerState extends ConsumerState<GraphExplorer> {
  final _searchController = TextEditingController();
  Timer? _debounce;
  List<GraphSearchResult> _results = [];
  GraphSearchResult? _selected;
  GraphRelatedResult? _related;
  String _typeFilter = 'all';
  bool _searching = false;
  bool _loadingRelated = false;

  final _typeFilters = ['all', 'function', 'type', 'file', 'document'];

  @override
  void dispose() {
    _searchController.dispose();
    _debounce?.cancel();
    super.dispose();
  }

  void _onSearchChanged(String query) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), () {
      if (query.length >= 2) _search(query);
    });
  }

  Future<void> _search(String query) async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    setState(() => _searching = true);
    try {
      final api = ref.read(apiClientProvider);
      final types = _typeFilter == 'all' ? null : [_typeFilter];
      final raw = await api.graphSearch(projectInfo.id, query, types: types);
      setState(() {
        _results = raw
            .map((e) => GraphSearchResult.fromJson(e as Map<String, dynamic>))
            .toList();
        _searching = false;
      });
    } catch (e) {
      setState(() => _searching = false);
    }
  }

  Future<void> _selectResult(GraphSearchResult result) async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    setState(() {
      _selected = result;
      _loadingRelated = true;
    });

    try {
      final api = ref.read(apiClientProvider);
      final json = await api.graphRelated(projectInfo.id, result.name);
      setState(() {
        _related = GraphRelatedResult.fromJson(json);
        _loadingRelated = false;
      });
    } catch (e) {
      setState(() => _loadingRelated = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final graphStats = ref.watch(graphStatsProvider);
    final size = MediaQuery.of(context).size;

    return Dialog(
      backgroundColor: GolemTheme.bgSurface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: GolemTheme.border),
      ),
      child: SizedBox(
        width: size.width * 0.9,
        height: size.height * 0.9,
        child: Column(
          children: [
            // Title bar
            Container(
              padding:
                  const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              decoration: const BoxDecoration(
                border:
                    Border(bottom: BorderSide(color: GolemTheme.border)),
              ),
              child: Row(
                children: [
                  const Icon(Icons.account_tree,
                      size: 18, color: GolemTheme.accent),
                  const SizedBox(width: 8),
                  const Text('Graph Explorer',
                      style: TextStyle(
                          fontSize: 15, fontWeight: FontWeight.w600)),
                  const Spacer(),
                  IconButton(
                    icon: const Icon(Icons.close, size: 18),
                    color: GolemTheme.textSecondary,
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
            ),
            // Content
            Expanded(
              child: Row(
                children: [
                  // Left pane — search
                  SizedBox(
                    width: 300,
                    child: _SearchPane(
                      controller: _searchController,
                      onChanged: _onSearchChanged,
                      results: _results,
                      selectedName: _selected?.name,
                      onSelect: _selectResult,
                      searching: _searching,
                      typeFilter: _typeFilter,
                      typeFilters: _typeFilters,
                      onTypeChanged: (t) {
                        setState(() => _typeFilter = t);
                        if (_searchController.text.length >= 2) {
                          _search(_searchController.text);
                        }
                      },
                    ),
                  ),
                  // Divider
                  const VerticalDivider(
                      width: 1, color: GolemTheme.border),
                  // Right pane — detail
                  Expanded(
                    child: _selected != null
                        ? _DetailPane(
                            result: _selected!,
                            related: _related,
                            loading: _loadingRelated,
                            onNavigate: (name) {
                              _searchController.text = name;
                              _search(name);
                            },
                          )
                        : _StatsOverview(stats: graphStats),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SearchPane extends StatelessWidget {
  final TextEditingController controller;
  final ValueChanged<String> onChanged;
  final List<GraphSearchResult> results;
  final String? selectedName;
  final ValueChanged<GraphSearchResult> onSelect;
  final bool searching;
  final String typeFilter;
  final List<String> typeFilters;
  final ValueChanged<String> onTypeChanged;

  const _SearchPane({
    required this.controller,
    required this.onChanged,
    required this.results,
    required this.selectedName,
    required this.onSelect,
    required this.searching,
    required this.typeFilter,
    required this.typeFilters,
    required this.onTypeChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(12),
          child: TextField(
            controller: controller,
            onChanged: onChanged,
            style: const TextStyle(fontSize: 13),
            decoration: const InputDecoration(
              hintText: 'Search symbols...',
              prefixIcon: Icon(Icons.search, size: 18),
            ),
          ),
        ),
        // Type filter chips
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: Wrap(
            spacing: 4,
            children: typeFilters
                .map((t) => ChoiceChip(
                      label: Text(
                          t == 'all'
                              ? 'All'
                              : t[0].toUpperCase() + t.substring(1),
                          style: const TextStyle(fontSize: 11)),
                      selected: typeFilter == t,
                      onSelected: (_) => onTypeChanged(t),
                      selectedColor:
                          GolemTheme.accent.withValues(alpha: 0.2),
                      backgroundColor: GolemTheme.bgPrimary,
                      side: const BorderSide(color: GolemTheme.border),
                      padding: EdgeInsets.zero,
                      labelPadding:
                          const EdgeInsets.symmetric(horizontal: 6),
                      visualDensity: VisualDensity.compact,
                    ))
                .toList(),
          ),
        ),
        const SizedBox(height: 8),
        if (searching)
          const Padding(
            padding: EdgeInsets.all(16),
            child: CircularProgressIndicator(
                strokeWidth: 2, color: GolemTheme.accent),
          ),
        Expanded(
          child: ListView.builder(
            itemCount: results.length,
            itemBuilder: (_, i) {
              final r = results[i];
              final isSelected = r.name == selectedName;
              return InkWell(
                onTap: () => onSelect(r),
                child: Container(
                  color: isSelected
                      ? GolemTheme.accent.withValues(alpha: 0.1)
                      : null,
                  padding: const EdgeInsets.symmetric(
                      horizontal: 12, vertical: 6),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          _TypeIcon(type: r.type),
                          const SizedBox(width: 6),
                          Expanded(
                            child: Text(r.name,
                                style: const TextStyle(fontSize: 13)),
                          ),
                          Text(
                            '${(r.score * 100).toStringAsFixed(0)}%',
                            style: GolemTheme.metaStyle(fontSize: 10),
                          ),
                        ],
                      ),
                      Padding(
                        padding: const EdgeInsets.only(left: 22),
                        child: Text(
                          '${r.path}${r.line > 0 ? ":${r.line}" : ""}',
                          style: GolemTheme.metaStyle(fontSize: 10),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ],
                  ),
                ),
              );
            },
          ),
        ),
      ],
    );
  }
}

class _TypeIcon extends StatelessWidget {
  final String type;
  const _TypeIcon({required this.type});

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (type) {
      'function' => (Icons.functions, GolemTheme.accent),
      'method' => (Icons.functions, GolemTheme.purple),
      'type' || 'class' || 'interface' => (Icons.data_object, GolemTheme.yellow),
      'file' => (Icons.insert_drive_file_outlined, GolemTheme.textSecondary),
      'document' || 'section' => (Icons.description_outlined, GolemTheme.green),
      _ => (Icons.code, GolemTheme.textSecondary),
    };
    return Icon(icon, size: 14, color: color);
  }
}

class _DetailPane extends StatelessWidget {
  final GraphSearchResult result;
  final GraphRelatedResult? related;
  final bool loading;
  final ValueChanged<String> onNavigate;

  const _DetailPane({
    required this.result,
    required this.related,
    required this.loading,
    required this.onNavigate,
  });

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header
          Row(
            children: [
              _TypeIcon(type: result.type),
              const SizedBox(width: 8),
              Text(result.name,
                  style: const TextStyle(
                      fontSize: 18, fontWeight: FontWeight.w600)),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '${result.type} \u2014 ${result.path}${result.line > 0 ? ":${result.line}" : ""}',
            style: GolemTheme.metaStyle(fontSize: 12),
          ),
          const SizedBox(height: 24),
          if (loading)
            const Center(
                child: CircularProgressIndicator(
                    strokeWidth: 2, color: GolemTheme.accent))
          else if (related != null) ...[
            _RelationSection(
              title: 'Calls',
              nodes: related!.nodes,
              edges:
                  related!.edges.where((e) => e.type == 'CALLS').toList(),
              sourceId: result.name,
              onNavigate: onNavigate,
              isOutbound: true,
            ),
            _RelationSection(
              title: 'Called by',
              nodes: related!.nodes,
              edges:
                  related!.edges.where((e) => e.type == 'CALLS').toList(),
              sourceId: result.name,
              onNavigate: onNavigate,
              isOutbound: false,
            ),
            _RelationSection(
              title: 'Imports',
              nodes: related!.nodes,
              edges: related!.edges
                  .where((e) => e.type == 'IMPORTS')
                  .toList(),
              sourceId: result.name,
              onNavigate: onNavigate,
              isOutbound: true,
            ),
          ],
        ],
      ),
    );
  }
}

class _RelationSection extends StatelessWidget {
  final String title;
  final List<GraphNode> nodes;
  final List<GraphEdge> edges;
  final String sourceId;
  final ValueChanged<String> onNavigate;
  final bool isOutbound;

  const _RelationSection({
    required this.title,
    required this.nodes,
    required this.edges,
    required this.sourceId,
    required this.onNavigate,
    required this.isOutbound,
  });

  @override
  Widget build(BuildContext context) {
    final relevantNodeIds = <String>{};
    for (final e in edges) {
      if (isOutbound) {
        relevantNodeIds.add(e.to);
      } else {
        relevantNodeIds.add(e.from);
      }
    }

    final filteredNodes = nodes
        .where(
            (n) => relevantNodeIds.contains(n.id) && n.name != sourceId)
        .toList();

    if (filteredNodes.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title,
            style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w600,
                color: GolemTheme.textSecondary)),
        const SizedBox(height: 4),
        ...filteredNodes.map((n) => InkWell(
              onTap: () => onNavigate(n.name),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 3),
                child: Row(
                  children: [
                    _TypeIcon(type: n.type),
                    const SizedBox(width: 6),
                    Text(n.name,
                        style: const TextStyle(
                            fontSize: 13, color: GolemTheme.accent)),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        '${n.path}${n.line > 0 ? ":${n.line}" : ""}',
                        style: GolemTheme.metaStyle(fontSize: 10),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  ],
                ),
              ),
            )),
        const SizedBox(height: 16),
      ],
    );
  }
}

class _StatsOverview extends StatelessWidget {
  final GraphStats? stats;
  const _StatsOverview({this.stats});

  @override
  Widget build(BuildContext context) {
    if (stats == null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.account_tree,
                size: 48, color: GolemTheme.border),
            const SizedBox(height: 12),
            const Text('No knowledge graph',
                style: TextStyle(fontSize: 15)),
            const SizedBox(height: 8),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: GolemTheme.bgPrimary,
                borderRadius: BorderRadius.circular(6),
              ),
              child: Text(
                'golem graph build && golem graph embed',
                style: GolemTheme.monoStyle(fontSize: 12),
              ),
            ),
          ],
        ),
      );
    }

    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Graph Overview',
              style:
                  TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
          const SizedBox(height: 4),
          const Text(
              'Select a symbol from search to explore relationships.',
              style: TextStyle(
                  fontSize: 13, color: GolemTheme.textSecondary)),
          const SizedBox(height: 24),
          Row(
            children: [
              _StatCard('Nodes', '${stats!.totalNodes}'),
              const SizedBox(width: 12),
              _StatCard('Edges', '${stats!.totalEdges}'),
              const SizedBox(width: 12),
              _StatCard('Embeddings', '${stats!.embeddingCount}'),
              const SizedBox(width: 12),
              _StatCard('Commits', '${stats!.commitCount}'),
            ],
          ),
          const SizedBox(height: 24),
          const Text('Node types',
              style:
                  TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
          const SizedBox(height: 8),
          ...stats!.nodeTypes.entries.map((e) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text(e.key,
                        style: const TextStyle(fontSize: 12)),
                    Text('${e.value}',
                        style: const TextStyle(
                            fontSize: 12,
                            color: GolemTheme.textSecondary)),
                  ],
                ),
              )),
        ],
      ),
    );
  }
}

class _StatCard extends StatelessWidget {
  final String label;
  final String value;
  const _StatCard(this.label, this.value);

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: GolemTheme.bgPrimary,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: GolemTheme.border),
        ),
        child: Column(
          children: [
            Text(value,
                style: const TextStyle(
                    fontSize: 20, fontWeight: FontWeight.w600)),
            const SizedBox(height: 2),
            Text(label,
                style: const TextStyle(
                    fontSize: 11, color: GolemTheme.textSecondary)),
          ],
        ),
      ),
    );
  }
}
