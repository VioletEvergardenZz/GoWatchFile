# -*- coding: utf-8 -*-

"""
使用OpenAI的API进行大模型的分析
"""

import os
import openai


def read_html_files(html_dir):
    """
    读取指定目录下的所有HTML文件内容
    
    Args:
        html_dir (str): HTML文件目录路径
        
    Returns:
        list: HTML文件内容列表
    """
    html_contents = []
    
    if html_dir and os.path.exists(html_dir):
        for filename in os.listdir(html_dir):
            if filename.endswith(".html"):
                file_path = os.path.join(html_dir, filename)
                if os.path.isfile(file_path):
                    with open(file_path, "r", encoding="utf-8") as f:
                        html_contents.append(f.read())
    
    return html_contents


def get_analysis_prompt():
    """
    获取分析提示词
    
    Returns:
        str: 分析提示词
    """
    return """
这是我用mat工具分析出来的java 程序oom时候的内存快照结果。请你作为一个资深的java工程师，对jvm有深刻的理解。现在帮我分析出来造成oom的主要原因。
然后使用一个带有css样式的html的格式返回, 可以适当附加一些简单的<script>，注意html的字符集要是utf-8， 只需要输出html，请勿添加额外内容， 结果需要使用中文。
报告内容需要提供如下内容：
1、根本原因分析
2、堆内存占用分布
3、对象引用链分析
4、导致OOM的操作链
5、OOM发生时的堆栈跟踪
6、内存泄漏模式识别
7、GC 行为分析
8、对象生命周期异常检测
9、可疑代码模块定位
10、关键词添加高亮
11、解决方案与优化建议
12、title与header使用: 基于AI自动化生成 Java应用OOM问题深度分析报告。 
13、header下使用p标签添加: 本回答由AI生成，内容仅供参考，请仔细甄别。
"""


def analyze_with_openai(html_text, prompt, model):
    """
    使用OpenAI API进行分析
    
    Args:
        html_text (str): HTML文件内容
        prompt (str): 分析提示词
        
    Returns:
        str: 分析结果
    """
    
    openai.api_key = os.environ.get("OPENAI_API_KEY")
    openai.api_base = os.environ.get("OPENAI_BASE_URL")
    
    completion = openai.ChatCompletion.create(
        model=model,
        messages=[
            {"role": "user", "content": html_text + prompt},
        ]
    )
    
    return completion.choices[0].message.content


def save_analysis_result(content, output_file="analysis.html"):
    """
    保存分析结果到文件
    
    Args:
        content (str): 分析结果内容
        output_file (str): 输出文件名
    """
    
    # 只保留以 <!DOCTYPE html> 开头，</html> 结尾的 html 内容
    import re
    match = re.search(r'(<!DOCTYPE html>.*?</html>)', content, re.DOTALL | re.IGNORECASE)
    if match:
        content = match.group(1)
    
    with open(output_file, "w", encoding="utf-8") as f:
        f.write(content)


def main():
    """主函数"""
    # 获取HTML目录
    html_dir = os.environ.get("HTML_DIR")
    out_put_file = os.environ.get("OUTPUT_FILE", "analysis.html")
    model = os.environ.get("MODEL", "deepseek-ai/DeepSeek-R1")
    # 读取HTML文件内容
    html_contents = read_html_files(html_dir)
    html_text = "\n".join(html_contents)
    
    # 获取分析提示词
    prompt = get_analysis_prompt()
    
    # 使用OpenAI进行分析
    analysis_result = analyze_with_openai(html_text, prompt, model)
    
    # 打印分析结果
    print(analysis_result)
    
    # 保存分析结果
    save_analysis_result(analysis_result, out_put_file)


if __name__ == "__main__":
    main()