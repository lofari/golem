(ns golem.dsl.engine.output-test
  (:require [clojure.test :refer [deftest is testing]]
            [golem.dsl.engine.output :as output]
            [clojure.java.io :as io]))

(deftest reads-session-output-edn
  (let [dir (io/file (System/getProperty "java.io.tmpdir")
                     (str "golem-test-" (System/currentTimeMillis)))
        _ (.mkdirs dir)
        output-file (io/file dir "session-output.edn")]
    (try
      (spit output-file (pr-str {:plan [{:step 1 :desc "do thing"}]}))
      (let [result (output/read-session-output dir)]
        (is (= [{:step 1 :desc "do thing"}] (:plan result))))
      (finally
        (.delete output-file)
        (.delete dir)))))

(deftest returns-empty-map-when-no-output
  (let [dir (io/file (System/getProperty "java.io.tmpdir")
                     (str "golem-test-" (System/currentTimeMillis)))]
    (.mkdirs dir)
    (try
      (is (= {} (output/read-session-output dir)))
      (finally
        (.delete dir)))))

(deftest detects-new-files
  (let [dir (io/file (System/getProperty "java.io.tmpdir")
                     (str "golem-test-" (System/currentTimeMillis)))
        _ (.mkdirs dir)]
    (try
      (let [before-files #{}
            _ (spit (io/file dir "new-file.go") "package main")
            after-files (output/scan-files dir)
            diff (output/file-diff before-files after-files)]
        (is (contains? (set diff) "new-file.go")))
      (finally
        (.delete (io/file dir "new-file.go"))
        (.delete dir)))))
