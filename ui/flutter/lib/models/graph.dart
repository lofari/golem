class GraphStats {
  final int totalNodes;
  final int totalEdges;
  final Map<String, int> nodeTypes;
  final Map<String, int> edgeTypes;
  final int embeddingCount;
  final String embedModel;
  final String lastIndexed;
  final int commitCount;
  final int authorCount;
  final int coChangeCount;
  final int executionCount;

  GraphStats({
    required this.totalNodes,
    required this.totalEdges,
    required this.nodeTypes,
    required this.edgeTypes,
    required this.embeddingCount,
    this.embedModel = '',
    this.lastIndexed = '',
    this.commitCount = 0,
    this.authorCount = 0,
    this.coChangeCount = 0,
    this.executionCount = 0,
  });

  factory GraphStats.fromJson(Map<String, dynamic> json) => GraphStats(
        totalNodes: json['totalNodes'] as int? ?? 0,
        totalEdges: json['totalEdges'] as int? ?? 0,
        nodeTypes: (json['nodeTypes'] as Map<String, dynamic>?)
                ?.map((k, v) => MapEntry(k, v as int)) ??
            {},
        edgeTypes: (json['edgeTypes'] as Map<String, dynamic>?)
                ?.map((k, v) => MapEntry(k, v as int)) ??
            {},
        embeddingCount: json['embeddingCount'] as int? ?? 0,
        embedModel: json['embedModel'] as String? ?? '',
        lastIndexed: json['lastIndexed'] as String? ?? '',
        commitCount: json['commitCount'] as int? ?? 0,
        authorCount: json['authorCount'] as int? ?? 0,
        coChangeCount: json['coChangeCount'] as int? ?? 0,
        executionCount: json['executionCount'] as int? ?? 0,
      );
}

class DiffSummary {
  final String baseRef;
  final List<FileDiff> files;
  final int totalAdded;
  final int totalRemoved;

  DiffSummary({
    this.baseRef = '',
    required this.files,
    this.totalAdded = 0,
    this.totalRemoved = 0,
  });

  factory DiffSummary.fromJson(Map<String, dynamic> json) => DiffSummary(
        baseRef: json['baseRef'] as String? ?? '',
        files: (json['files'] as List<dynamic>?)
                ?.map((e) => FileDiff.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        totalAdded: json['totalAdded'] as int? ?? 0,
        totalRemoved: json['totalRemoved'] as int? ?? 0,
      );
}

class FileDiff {
  final String path;
  final int additions;
  final int deletions;
  String? patch; // loaded on demand

  FileDiff({
    required this.path,
    this.additions = 0,
    this.deletions = 0,
    this.patch,
  });

  factory FileDiff.fromJson(Map<String, dynamic> json) => FileDiff(
        path: json['path'] as String? ?? '',
        additions: json['additions'] as int? ?? 0,
        deletions: json['deletions'] as int? ?? 0,
      );
}

class ContextMapResult {
  final String task;
  final List<ContextSymbol> symbols;

  ContextMapResult({required this.task, required this.symbols});

  factory ContextMapResult.fromJson(Map<String, dynamic> json) =>
      ContextMapResult(
        task: json['Task'] as String? ?? '',
        symbols: (json['Symbols'] as List<dynamic>?)
                ?.map((e) => ContextSymbol.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
      );
}

class ContextSymbol {
  final String name;
  final String kind;
  final String path;
  final int line;
  final double score;
  final List<String> relations;

  ContextSymbol({
    required this.name,
    required this.kind,
    required this.path,
    this.line = 0,
    this.score = 0,
    this.relations = const [],
  });

  factory ContextSymbol.fromJson(Map<String, dynamic> json) => ContextSymbol(
        name: json['Name'] as String? ?? '',
        kind: json['Kind'] as String? ?? '',
        path: json['Path'] as String? ?? '',
        line: json['Line'] as int? ?? 0,
        score: (json['Score'] as num?)?.toDouble() ?? 0,
        relations:
            (json['Relations'] as List<dynamic>?)?.cast<String>() ?? [],
      );
}

class GraphSearchResult {
  final String name;
  final String path;
  final int line;
  final String type;
  final double score;

  GraphSearchResult({
    required this.name,
    required this.path,
    this.line = 0,
    required this.type,
    this.score = 0,
  });

  factory GraphSearchResult.fromJson(Map<String, dynamic> json) =>
      GraphSearchResult(
        name: json['name'] as String? ?? '',
        path: json['path'] as String? ?? '',
        line: json['line'] as int? ?? 0,
        type: json['type'] as String? ?? '',
        score: (json['score'] as num?)?.toDouble() ?? 0,
      );
}

class GraphNode {
  final String id;
  final String type;
  final String name;
  final String path;
  final int line;

  GraphNode({
    required this.id,
    required this.type,
    required this.name,
    required this.path,
    this.line = 0,
  });

  factory GraphNode.fromJson(Map<String, dynamic> json) => GraphNode(
        id: json['id'] as String? ?? '',
        type: json['type'] as String? ?? '',
        name: json['name'] as String? ?? '',
        path: json['path'] as String? ?? '',
        line: json['line'] as int? ?? 0,
      );
}

class GraphEdge {
  final String from;
  final String to;
  final String type;

  GraphEdge({required this.from, required this.to, required this.type});

  factory GraphEdge.fromJson(Map<String, dynamic> json) => GraphEdge(
        from: json['from'] as String? ?? '',
        to: json['to'] as String? ?? '',
        type: json['type'] as String? ?? '',
      );
}

class GraphRelatedResult {
  final List<GraphNode> nodes;
  final List<GraphEdge> edges;

  GraphRelatedResult({required this.nodes, required this.edges});

  factory GraphRelatedResult.fromJson(Map<String, dynamic> json) =>
      GraphRelatedResult(
        nodes: (json['nodes'] as List<dynamic>?)
                ?.map((e) => GraphNode.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        edges: (json['edges'] as List<dynamic>?)
                ?.map((e) => GraphEdge.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
      );
}
