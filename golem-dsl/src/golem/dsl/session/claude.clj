(ns golem.dsl.session.claude
  "Claude Code session adapter. Calls golem session."
  (:require [golem.dsl.session.protocol :as proto]
            [clojure.java.io :as io]
            [clojure.edn :as edn])
  (:import [java.lang ProcessBuilder]
           [java.io File]))

(defn build-command
  "Build the golem session command args."
  [adapter prompt-file working-dir opts]
  (let [{:keys [golem-binary sandbox plugin-dirs max-turns model
                mcp no-lsp]} adapter]
    (cond-> [(or golem-binary "golem") "session"
             "--prompt" prompt-file
             "--dir" working-dir
             "--max-turns" (str (or max-turns 200))]
      sandbox     (conj "--sandbox")
      model       (into ["--model" model])
      (false? mcp) (conj "--mcp=false")
      no-lsp      (conj "--no-lsp")
      plugin-dirs (into (mapcat #(vector "--plugin-dir" %) plugin-dirs)))))

(defrecord ClaudeAdapter [golem-binary sandbox plugin-dirs max-turns model
                          mcp no-lsp]
  proto/SessionAdapter
  (spawn [this prompt working-dir opts]
    (let [prompt-file (File/createTempFile "golem-prompt-" ".md")
          _ (spit prompt-file prompt)
          cmd (build-command this (.getAbsolutePath prompt-file) working-dir opts)
          pb (doto (ProcessBuilder. ^java.util.List cmd)
               (.directory (io/file working-dir))
               (.redirectErrorStream true))
          process (.start pb)]
      {:process process
       :prompt-file prompt-file
       :working-dir working-dir}))

  (await-result [this handle timeout-ms]
    (let [process (:process handle)
          completed (.waitFor process
                             (or timeout-ms 600000)
                             java.util.concurrent.TimeUnit/MILLISECONDS)]
      (if completed
        {:exit-code (.exitValue process)}
        (do (.destroyForcibly process)
            {:exit-code -1 :timeout true}))))

  (read-output [this handle]
    (let [output-file (io/file (:working-dir handle) "session-output.edn")]
      (if (.exists output-file)
        (edn/read-string (slurp output-file))
        {}))))

(defn make-adapter [opts]
  (map->ClaudeAdapter opts))
